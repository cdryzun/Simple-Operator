/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	v1 "k8s.io/api/networking/v1"

	appv1alpha1 "github.com/cdryzun/Simple-Operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// AppServiceReconciler reconciles a AppService object
type AppServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=app.github.com,resources=appservices,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=app.github.com,resources=appservices/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=app.github.com,resources=appservices/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the AppService object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *AppServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reqLogger := log.Log.WithValues("Request.Namespace", req.Namespace, "Request.Name", req.Name)
	reqLogger.Info("Reconciling AppService")

	// Fetch the AppService instance
	instance := &appv1alpha1.AppService{}
	err := r.Get(context.TODO(), req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return ctrl.Result{}, err
	}

	// If it does not exist, create associated resources
	// If it exists, determine if an update is needed
	// If an update is needed, update directly
	// If no update is needed, return normally

	deploy := &appsv1.Deployment{}
	fmt.Println(deploy)
	if err := r.Get(context.TODO(), req.NamespacedName, deploy); err != nil && errors.IsNotFound(err) {
		// Create associated resources
		// 1. Create Deploy
		deploy := NewDeploy(instance)
		if err := r.Create(context.TODO(), deploy); err != nil {
			return ctrl.Result{}, err
		}
		// 2. Create Service
		service := NewService(instance)
		if err := r.Create(context.TODO(), service); err != nil {
			return ctrl.Result{}, err
		}

		// 2.1  Create Ingress
		if instance.Spec.Hostname != "" {
			ingress := NewIngress(instance)
			if err := r.Client.Create(context.TODO(), ingress); err != nil {
				return ctrl.Result{}, err
			}
		}

		// 3. Associated Annotations
		data, _ := json.Marshal(instance.Spec)
		if instance.Annotations != nil {
			instance.Annotations["spec"] = string(data)
		} else {
			instance.Annotations = map[string]string{"spec": string(data)}
		}
		if err := r.Update(context.TODO(), instance); err != nil {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, nil
	}

	oldspec := appv1alpha1.AppServiceSpec{}
	if err := json.Unmarshal([]byte(instance.Annotations["spec"]), &oldspec); err != nil {
		return ctrl.Result{}, err
	}

	if !reflect.DeepEqual(instance.Spec, oldspec) {
		// Update associated resources
		newDeploy := NewDeploy(instance)
		oldDeploy := &appsv1.Deployment{}
		if err := r.Get(context.TODO(), req.NamespacedName, oldDeploy); err != nil {
			return ctrl.Result{}, err
		}
		oldDeploy.Spec = newDeploy.Spec
		if err := r.Update(context.TODO(), oldDeploy); err != nil {
			return ctrl.Result{}, err
		}

		newService := NewService(instance)
		oldService := &corev1.Service{}
		if err := r.Get(context.TODO(), req.NamespacedName, oldService); err != nil {
			return ctrl.Result{}, err
		}
		oldService.Spec = newService.Spec
		if err := r.Update(context.TODO(), oldService); err != nil {
			return ctrl.Result{}, err
		}

		if instance.Spec.Hostname != "" {
			newIngress := NewIngress(instance)
			r.Client.Create(context.TODO(), newIngress)

			oldIngress := &v1.Ingress{}
			if err := r.Get(context.TODO(), req.NamespacedName, oldIngress); err != nil {
				return ctrl.Result{}, err
			}
			oldIngress.Spec = newIngress.Spec
			if err := r.Update(context.TODO(), oldIngress); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			// Delete Ingress
			oldIngress := &v1.Ingress{
				ObjectMeta: ctrl.ObjectMeta{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			}
			if err := r.Delete(context.Background(), oldIngress); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
	}

	return reconcile.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AppServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appv1alpha1.AppService{}).
		Complete(r)
}

// Implement function functions
func NewDeploy(instance *appv1alpha1.AppService) *appsv1.Deployment {
	labels := map[string]string{
		"app": instance.Name,
	}
	return &appsv1.Deployment{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					instance,
					schema.GroupVersionKind{
						Group:   appv1alpha1.SchemeBuilder.GroupVersion.Group,
						Version: appv1alpha1.SchemeBuilder.GroupVersion.Version,
						Kind:    "AppService",
					},
				),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: instance.Spec.Size,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: newContainers(instance),
				},
			},
		},
	}
}

func NewService(instance *appv1alpha1.AppService) *corev1.Service {
	labels := map[string]string{
		"app": instance.Name,
	}
	return &corev1.Service{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					instance,
					schema.GroupVersionKind{
						Group:   appv1alpha1.SchemeBuilder.GroupVersion.Group,
						Version: appv1alpha1.SchemeBuilder.GroupVersion.Version,
						Kind:    "AppService",
					},
				),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:  corev1.ServiceTypeNodePort,
			Ports: instance.Spec.Ports,
			Selector: map[string]string{
				"app": instance.Name,
			},
		},
	}
}

func newContainers(instance *appv1alpha1.AppService) []corev1.Container {
	var containerPorts []corev1.ContainerPort
	for _, svcPort := range instance.Spec.Ports {
		cport := corev1.ContainerPort{}
		cport.ContainerPort = svcPort.TargetPort.IntVal
		containerPorts = append(containerPorts, cport)
	}

	container := corev1.Container{
		Name:            instance.Name,
		Image:           instance.Spec.Image,
		Ports:           containerPorts,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             instance.Spec.Envs,
	}

	if instance.Spec.Resources != nil {
		if instance.Spec.Resources.Requests != nil {
			container.Resources.Requests = instance.Spec.Resources.Requests
		}
		if instance.Spec.Resources.Limits != nil {
			container.Resources.Limits = instance.Spec.Resources.Limits
		}
	}

	return []corev1.Container{
		container,
	}
}

func NewIngress(instance *appv1alpha1.AppService) *v1.Ingress {
	return &v1.Ingress{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/rewrite-target": "/",
			},
			Labels: map[string]string{
				"app": instance.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(
					instance,
					schema.GroupVersionKind{
						Group:   appv1alpha1.SchemeBuilder.GroupVersion.Group,
						Version: appv1alpha1.SchemeBuilder.GroupVersion.Version,
						Kind:    "AppService",
					},
				)},
		},
		Spec: v1.IngressSpec{
			Rules: []v1.IngressRule{
				{
					Host: instance.Spec.Hostname,
					IngressRuleValue: v1.IngressRuleValue{
						HTTP: &v1.HTTPIngressRuleValue{
							Paths: []v1.HTTPIngressPath{
								{
									Path: "/",
									PathType: func() *v1.PathType {
										pathType := v1.PathTypePrefix
										return &pathType
									}(),
									Backend: v1.IngressBackend{
										Service: &v1.IngressServiceBackend{
											Name: instance.Name,
											Port: v1.ServiceBackendPort{
												Number: instance.Spec.Ports[0].TargetPort.IntVal,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
