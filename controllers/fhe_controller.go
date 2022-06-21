/*
Copyright 2022.

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
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	fhev1alpha1 "github.com/dcurran90/fhe-operator/api/v1alpha1"
)

// FHEReconciler reconciles a FHE object
type FHEReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=fhe.redhat.com,resources=fhes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=fhe.redhat.com,resources=fhes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=fhe.redhat.com,resources=fhes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the FHE object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *FHEReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	Log := log.FromContext(ctx)

	fhe := &fhev1alpha1.FHE{}
	err := r.Client.Get(ctx, req.NamespacedName, fhe)
	if err != nil {
		if errors.IsNotFound(err) {
			Log.Info("Object not found.")
			return ctrl.Result{}, nil
		}
		Log.Error(err, "Error getting fhe object")
		return ctrl.Result{Requeue: true}, err
	}
	Log.Info("Got new todo", "todo", fhe.ObjectMeta.Name)

	// reconcile volume
	configVolume := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: fhe.GetName(), Namespace: fhe.GetNamespace()}, configVolume)
	if err != nil {
		if errors.IsNotFound(err) {
			fileName := "config/samples/manifests/volume.yaml"
			r.applyManifest(ctx, req, fhe, configVolume, fileName, toolkitConfigVolume)
		} else {
			return ctrl.Result{Requeue: true}, err
		}
	}
	localVolume := &corev1.PersistentVolumeClaim{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: fhe.GetName(), Namespace: fhe.GetNamespace()}, localVolume)
	if err != nil {
		if errors.IsNotFound(err) {
			fileName := "config/samples/manifests/volume.yaml"
			r.applyManifest(ctx, req, fhe, localVolume, fileName, toolkitLocalVolume)
		} else {
			return ctrl.Result{Requeue: true}, err
		}
	}

	// reconcile deployment
	deployment := &appsv1.Deployment{}

	err = r.Client.Get(ctx, types.NamespacedName{Name: fhe.GetName(), Namespace: fhe.GetNamespace()}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			fileName := "config/samples/manifests/deployment.yaml"
			r.applyManifest(ctx, req, fhe, deployment, fileName, toolkitDeployment)
		} else {
			return ctrl.Result{Requeue: true}, err
		}
		// TODO: should we update then?
	}

	// reconcile service
	service := &corev1.Service{}

	err = r.Client.Get(ctx, types.NamespacedName{Name: fhe.GetName(), Namespace: fhe.GetNamespace()}, service)
	if err != nil {
		if errors.IsNotFound(err) {
			fileName := "config/samples/manifests/service.yaml"
			r.applyManifest(ctx, req, fhe, service, fileName, toolkitService)
		} else {
			return ctrl.Result{Requeue: true}, err
		}
		// TODO: should we update then?
	}

	// reconcile ingress
	ingress := &networkingv1.Ingress{}

	err = r.Client.Get(ctx, types.NamespacedName{Name: fhe.GetName(), Namespace: fhe.GetNamespace()}, ingress)
	if err != nil {
		if errors.IsNotFound(err) {
			fileName := "config/samples/manifests/ingress.yaml"
			r.applyManifest(ctx, req, fhe, ingress, fileName, toolkitIngress)
		} else {
			return ctrl.Result{Requeue: true}, err
		}
		// TODO: should we update then?
	}

	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *FHEReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fhev1alpha1.FHE{}).
		Complete(r)
}

func (r *FHEReconciler) applyManifest(ctx context.Context, req ctrl.Request, fhe *fhev1alpha1.FHE, obj client.Object, fileName string, customizer func(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object)) error {
	Log := log.FromContext(ctx)

	binary, err := os.ReadFile(fileName)
	if err != nil {
		Log.Error(err, fmt.Sprintf("Couldn't read manifest file for: %s", fileName))
		return err
	}

	if err = yamlutil.Unmarshal(binary, &obj); err != nil {
		Log.Error(err, fmt.Sprintf("Couldn't unmarshall yaml file for: %s", fileName))
		return err
	}

	obj.SetNamespace(fhe.GetNamespace())
	obj.SetName(fhe.GetName())
	controllerutil.SetControllerReference(fhe, obj, r.Scheme)
	customizer(ctx, fhe, obj)

	err = r.Client.Create(ctx, obj)
	if err != nil {
		// Creation failed
		Log.Error(err, "Failed to create new Object. ", "Namespace : ", obj.GetNamespace(), " Name : ", obj.GetName())
		return err
	}

	return nil
}

func toolkitDeployment(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object) {
	Log := log.FromContext(ctx)
	Log.Info("Customizing deployment")

	dep := obj.(*appsv1.Deployment)
	dep.Spec.Template.Spec.Containers[0].Image = "docker.io/ibmcom/helayers-pylab"
	// secondContainer := dep.Spec.Template.Spec.Containers[0].DeepCopy()
	// secondContainer.Name = "fhe-pylab"
	// secondContainer.Image = "docker.io/ibmcom/helayers-pylab"
	// dep.Spec.Template.Spec.Containers = append(dep.Spec.Template.Spec.Containers, *secondContainer)
}

func toolkitConfigVolume(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object) {
	Log := log.FromContext(ctx)
	Log.Info("Customizing volume")
	obj.SetName(obj.GetName() + "-config")

	vol := obj.(*corev1.PersistentVolumeClaim)

	vol.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceStorage: resource.MustParse("5Gi"),
	}
}

func toolkitLocalVolume(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object) {
	Log := log.FromContext(ctx)
	Log.Info("Customizing volume")
	obj.SetName(obj.GetName() + "-local")

	vol := obj.(*corev1.PersistentVolumeClaim)

	vol.Spec.Resources.Requests = corev1.ResourceList{
		corev1.ResourceStorage: resource.MustParse("10Gi"),
	}
}

func toolkitService(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object) {
	Log := log.FromContext(ctx)
	Log.Info("Customizing service")

	svc := obj.(*corev1.Service)
	labels := map[string]string{
		"app": "fhe",
	}
	svc.Spec.Selector = labels
}

func toolkitIngress(ctx context.Context, fhe *fhev1alpha1.FHE, obj client.Object) {
	Log := log.FromContext(ctx)
	Log.Info("Customizing ingress")

	ingress := obj.(*networkingv1.Ingress)
	Log.Info(ingress.Name)
	ingress.Spec.Rules[0].Host = ingress.Name + "." + ingress.Namespace + ".apps" + appDomainName()
}
