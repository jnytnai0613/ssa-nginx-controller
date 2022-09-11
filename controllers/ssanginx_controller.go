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
	"log"
	"strings"

	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	ssanginxv1 "github.com/jnytnai0613/ssa-nginx-controller/api/v1"
)

// SSANginxReconciler reconciles a SSANginx object
type SSANginxReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

var kclientset *kubernetes.Clientset

func init() {
	kclientset = getClient()
}

func getClient() *kubernetes.Clientset {
	kubeconfig := ctrl.GetConfigOrDie()
	kclientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatal(err)
	}

	return kclientset
}

func createOwnerReferences(ssanginx ssanginxv1.SSANginx, scheme *runtime.Scheme, log logr.Logger) (*metav1apply.OwnerReferenceApplyConfiguration, error) {
	gvk, err := apiutil.GVKForObject(&ssanginx, scheme)
	if err != nil {
		log.Error(err, "Unable get GVK")
		return nil, err
	}

	owner := metav1apply.OwnerReference().
		WithAPIVersion(gvk.GroupVersion().String()).
		WithKind(gvk.Kind).
		WithName(ssanginx.GetName()).
		WithUID(ssanginx.GetUID()).
		WithBlockOwnerDeletion(true).
		WithController(true)

	return owner, nil
}

func (r *SSANginxReconciler) applyConfigMap(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		configMap       corev1.ConfigMap
		configMapClient = kclientset.CoreV1().ConfigMaps("ssa-nginx-controller-system")
	)

	nextConfigMapApplyConfig := corev1apply.ConfigMap(ssanginx.Spec.ConfigMapName, "ssa-nginx-controller-system").
		WithData(ssanginx.Spec.ConfigMapData)

	owner, err := createOwnerReferences(ssanginx, r.Scheme, log)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextConfigMapApplyConfig.WithOwnerReferences(owner)

	if err := r.Get(ctx, client.ObjectKey{Namespace: "ssa-nginx-controller-system", Name: ssanginx.Spec.ConfigMapName}, &configMap); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	currConfigMapApplyConfig, err := corev1apply.ExtractConfigMap(&configMap, fieldMgr)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(currConfigMapApplyConfig, nextConfigMapApplyConfig) {
		return nil
	}

	applied, err := configMapClient.Apply(ctx, nextConfigMapApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Configmap Applied: %s", applied.GetName()))

	return nil
}

func (r *SSANginxReconciler) applyDeployment(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		deployment       appsv1.Deployment
		deploymentClient = kclientset.AppsV1().Deployments("ssa-nginx-controller-system")
		labels           = map[string]string{"apps": "ssa-nginx"}
		podTemplate      *corev1apply.PodTemplateSpecApplyConfiguration
	)

	nextDeploymentApplyConfig := appsv1apply.Deployment(ssanginx.Spec.DeploymentName, "ssa-nginx-controller-system").
		WithSpec(appsv1apply.DeploymentSpec().
			WithSelector(metav1apply.LabelSelector().
				WithMatchLabels(labels)))

	if ssanginx.Spec.DeploymentSpec.Replicas != nil {
		replicas := *ssanginx.Spec.DeploymentSpec.Replicas
		nextDeploymentApplyConfig.Spec.WithReplicas(replicas)
	}

	if ssanginx.Spec.DeploymentSpec.Strategy != nil {
		types := *ssanginx.Spec.DeploymentSpec.Strategy.Type
		rollingUpdate := ssanginx.Spec.DeploymentSpec.Strategy.RollingUpdate
		nextDeploymentApplyConfig.Spec.WithStrategy(appsv1apply.DeploymentStrategy().
			WithType(types).
			WithRollingUpdate(rollingUpdate))
	}

	podTemplate = ssanginx.Spec.DeploymentSpec.Template
	podTemplate.WithLabels(labels)

	for i, _ := range podTemplate.Spec.Containers {
		s := strings.Split(*podTemplate.Spec.Containers[i].Image, ":")
		if s[0] == "nginx" {
			podTemplate.Spec.Containers[i].WithVolumeMounts(
				corev1apply.VolumeMount().
					WithName("conf").
					WithMountPath("/etc/nginx/conf.d/"),
				corev1apply.VolumeMount().
					WithName("index").
					WithMountPath("/usr/share/nginx/html/"))

			break
		}
	}

	podTemplate.Spec.WithVolumes(
		corev1apply.Volume().
			WithName("conf").
			WithConfigMap(corev1apply.ConfigMapVolumeSource().
				WithName(ssanginx.Spec.ConfigMapName).
				WithItems(corev1apply.KeyToPath().
					WithKey("default.conf").
					WithPath("default.conf"))),
		corev1apply.Volume().
			WithName("index").
			WithConfigMap(corev1apply.ConfigMapVolumeSource().
				WithName(ssanginx.Spec.ConfigMapName).
				WithItems(corev1apply.KeyToPath().
					WithKey("mod-index.html").
					WithPath("mod-index.html"))))

	nextDeploymentApplyConfig.Spec.WithTemplate(podTemplate)

	owner, err := createOwnerReferences(ssanginx, r.Scheme, log)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextDeploymentApplyConfig.WithOwnerReferences(owner)

	if err := r.Get(ctx, client.ObjectKey{Namespace: "ssa-nginx-controller-system", Name: ssanginx.Spec.DeploymentName}, &deployment); err != nil {
		if !errors.IsNotFound(err) {
			return err
		}
	}

	currDeploymentApplyConfig, err := appsv1apply.ExtractDeployment(&deployment, fieldMgr)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(currDeploymentApplyConfig, nextDeploymentApplyConfig) {
		return nil
	}

	applied, err := deploymentClient.Apply(ctx, nextDeploymentApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Deployment Applied: %s", applied.GetName()))

	return nil
}

//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *SSANginxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		fieldMgr = "ssanginx-fieldmanager"
		log      = r.Log.WithValues("ssanginx", req.NamespacedName)
		ssanginx ssanginxv1.SSANginx
	)

	if err := r.Get(ctx, req.NamespacedName, &ssanginx); err != nil {
		log.Error(err, "unable to fetch CR SSANginx")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Create Configmap
	// Generate default.conf and index.html
	if err := r.applyConfigMap(ctx, fieldMgr, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	// Create Deployment
	if err := r.applyDeployment(ctx, fieldMgr, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SSANginxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ssanginxv1.SSANginx{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
