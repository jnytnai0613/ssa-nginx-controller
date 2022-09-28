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
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	metav1apply "k8s.io/client-go/applyconfigurations/meta/v1"
	networkv1apply "k8s.io/client-go/applyconfigurations/networking/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	ssanginxv1 "github.com/jnytnai0613/ssa-nginx-controller/api/v1"
	"github.com/jnytnai0613/ssa-nginx-controller/pkg/constants"
	"github.com/jnytnai0613/ssa-nginx-controller/pkg/pki"
)

// SSANginxReconciler reconciles a SSANginx object
type SSANginxReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
	Scheme   *runtime.Scheme
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

func (r *SSANginxReconciler) deleteOwnedResources(ctx context.Context, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		configMaps  corev1.ConfigMapList
		deployments appsv1.DeploymentList
		ingresses   networkv1.IngressList
		services    corev1.ServiceList
	)

	if err := r.List(ctx, &configMaps, client.InNamespace(ssanginx.GetNamespace()),
		client.MatchingFields(map[string]string{constants.IndexOwnerKey: ssanginx.GetName()})); err != nil {
		return err
	}
	if err := r.List(ctx, &deployments, client.InNamespace(ssanginx.GetNamespace()),
		client.MatchingFields(map[string]string{constants.IndexOwnerKey: ssanginx.GetName()})); err != nil {
		return err
	}
	if err := r.List(ctx, &services, client.InNamespace(ssanginx.GetNamespace()),
		client.MatchingFields(map[string]string{constants.IndexOwnerKey: ssanginx.GetName()})); err != nil {
		return err
	}
	if err := r.List(ctx, &ingresses, client.InNamespace(ssanginx.GetNamespace()),
		client.MatchingFields(map[string]string{constants.IndexOwnerKey: ssanginx.GetName()})); err != nil {
		return err
	}

	for _, configmap := range configMaps.Items {
		if configmap.GetName() == ssanginx.Spec.ConfigMapName {
			continue
		}

		if err := r.Delete(ctx, &configmap); err != nil {
			// If ConfigMap is renamed, this function may be called
			// almost simultaneously because the Manager detects changes
			// in ConfigMap and Deployment (since Configmap is mounted).
			// In that case, if the resource is deleted first,
			// a Not Found error will occur.
			// Returns nil if a Not Found error occurs.
			return client.IgnoreNotFound(err)
		}

		log.Info(fmt.Sprintf("delete ConfigMap resource: %s", configmap.GetName()))
		r.Recorder.Eventf(&configmap, corev1.EventTypeNormal, "Deleted", "Deleted ConfigMap %q", configmap.GetName())
	}

	for _, deployment := range deployments.Items {
		if deployment.GetName() == ssanginx.Spec.DeploymentName {
			continue
		}

		if err := r.Delete(ctx, &deployment); err != nil {
			return err
		}

		log.Info(fmt.Sprintf("delete Deployment resource: %s", deployment.GetName()))
		r.Recorder.Eventf(&deployment, corev1.EventTypeNormal, "Deleted", "Deleted Deployment %q", deployment.GetName())
	}

	for _, service := range services.Items {
		if service.GetName() == ssanginx.Spec.ServiceName {
			continue
		}

		if err := r.Delete(ctx, &service); err != nil {
			return err
		}

		log.Info(fmt.Sprintf("delete Service resource: %s", service.GetName()))
		r.Recorder.Eventf(&service, corev1.EventTypeNormal, "Deleted", "Deleted Service %q", service.GetName())
	}

	for _, ingress := range ingresses.Items {
		if ingress.GetName() == ssanginx.Spec.IngressName {
			continue
		}

		if err := r.Delete(ctx, &ingress); err != nil {
			return err
		}

		log.Info(fmt.Sprintf("delete Ingress resource: %s", ingress.GetName()))
		r.Recorder.Eventf(&ingress, corev1.EventTypeNormal, "Deleted", "Deleted Ingress %q", ingress.GetName())
	}

	return nil

}

func createInitContainers() []*corev1apply.ContainerApplyConfiguration {
	var initContainers []*corev1apply.ContainerApplyConfiguration
	i := corev1apply.Container().
		WithName(constants.InitConatainerName).
		WithImage(constants.InitConatainerImage).
		WithCommand(
			"sh",
			"-c",
			constants.InitCommand).
		WithVolumeMounts(
			corev1apply.VolumeMount().
				WithName(constants.EmptyDirVolumeName).
				WithMountPath(constants.EmptyDirVolumeMountPath))
	initContainers = append(initContainers, i)

	return initContainers
}

// Create OwnerReference with CR as Owner
func createOwnerReferences(log logr.Logger, ssanginx ssanginxv1.SSANginx, scheme *runtime.Scheme) (*metav1apply.OwnerReferenceApplyConfiguration, error) {
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
		configMapClient = kclientset.CoreV1().ConfigMaps(constants.Namespace)
	)

	nextConfigMapApplyConfig := corev1apply.ConfigMap(ssanginx.Spec.ConfigMapName, constants.Namespace).
		WithData(ssanginx.Spec.ConfigMapData)

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextConfigMapApplyConfig.WithOwnerReferences(owner)

	// Difference Check at Client-Side
	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: ssanginx.Spec.ConfigMapName}, &configMap); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
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
		configmap        corev1.ConfigMap
		deployment       appsv1.Deployment
		deploymentClient = kclientset.AppsV1().Deployments(constants.Namespace)
		labels           = map[string]string{"apps": "nginx"}
		indexKey         string
	)

	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: ssanginx.Spec.ConfigMapName}, &configmap); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
		if !errors.IsNotFound(err) {
			return err
		}
	}

	for key := range configmap.Data {
		if strings.Contains(key, "htm") {
			indexKey = key
		}
	}

	// It is possible that there is no Data in ConfigMap at this point.
	// If Data does not exist, the function exits to retry.
	if indexKey == "" {
		log.Info("index key is empty")
		return nil
	}

	nextDeploymentApplyConfig := appsv1apply.Deployment(ssanginx.Spec.DeploymentName, constants.Namespace).
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

	podTemplate := ssanginx.Spec.DeploymentSpec.Template
	podTemplate.WithLabels(labels)
	initContainers := createInitContainers()
	podTemplate.Spec.WithInitContainers(initContainers...)

	for i, _ := range podTemplate.Spec.Containers {
		s := strings.Split(*podTemplate.Spec.Containers[i].Image, ":")
		if s[0] == constants.CompareImageName {
			podTemplate.Spec.Containers[i].
				WithCommand(
					"bash",
					"-c",
					constants.ContainerCommand).
				WithVolumeMounts(
					corev1apply.VolumeMount().
						WithName(constants.ConfVolumeName).
						WithMountPath(constants.ConfVolumeMountPath),
					corev1apply.VolumeMount().
						WithName(constants.IndexVolumeName).
						WithMountPath(constants.IndexVolumeMountPath),
					corev1apply.VolumeMount().
						WithName(constants.EmptyDirVolumeName).
						WithMountPath(constants.EmptyDirVolumeMountPath))
			break
		}
	}

	podTemplate.Spec.WithVolumes(
		corev1apply.Volume().
			WithName(constants.ConfVolumeName).
			WithConfigMap(corev1apply.ConfigMapVolumeSource().
				WithName(ssanginx.Spec.ConfigMapName).
				WithItems(corev1apply.KeyToPath().
					WithKey(constants.ConfVolumeKeyPath).
					WithPath(constants.ConfVolumeKeyPath))),
		corev1apply.Volume().
			WithName(constants.IndexVolumeName).
			WithConfigMap(corev1apply.ConfigMapVolumeSource().
				WithName(ssanginx.Spec.ConfigMapName).
				WithItems(corev1apply.KeyToPath().
					WithKey(indexKey).
					WithPath(indexKey))),
		corev1apply.Volume().
			WithName(constants.EmptyDirVolumeName).
			WithEmptyDir(nil))

	nextDeploymentApplyConfig.Spec.WithTemplate(podTemplate)

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextDeploymentApplyConfig.WithOwnerReferences(owner)

	// Difference Check at Client-Side
	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: ssanginx.Spec.DeploymentName}, &deployment); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
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

func (r *SSANginxReconciler) applyService(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		service       corev1.Service
		serviceClient = kclientset.CoreV1().Services(constants.Namespace)
		labels        = map[string]string{"apps": "nginx"}
	)

	nextServiceApplyConfig := corev1apply.Service(ssanginx.Spec.ServiceName, constants.Namespace).
		WithSpec((*corev1apply.ServiceSpecApplyConfiguration)(ssanginx.Spec.ServiceSpec).
			WithSelector(labels))

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextServiceApplyConfig.WithOwnerReferences(owner)

	// Difference Check at Client-Side
	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: ssanginx.Spec.ServiceName}, &service); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
		if !errors.IsNotFound(err) {
			return err
		}
	}
	currServiceApplyConfig, err := corev1apply.ExtractService(&service, fieldMgr)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(currServiceApplyConfig, nextServiceApplyConfig) {
		return nil
	}

	applied, err := serviceClient.Apply(ctx, nextServiceApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Service Applied: %s", applied.GetName()))

	return nil
}

func (r *SSANginxReconciler) applyIngress(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		annotateRewriteTarget = map[string]string{"nginx.ingress.kubernetes.io/rewrite-target": "/"}
		annotateVerifyClient  = map[string]string{"nginx.ingress.kubernetes.io/auth-tls-verify-client": "on"}
		annotateTlsSecret     = map[string]string{"nginx.ingress.kubernetes.io/auth-tls-secret": fmt.Sprintf("%s/%s", constants.Namespace, constants.IngressSecretName)}
		ingress               networkv1.Ingress
		ingressClient         = kclientset.NetworkingV1().Ingresses(constants.Namespace)
		secrets               corev1.SecretList
	)

	nextIngressApplyConfig := networkv1apply.Ingress(ssanginx.Spec.IngressName, constants.Namespace).
		WithAnnotations(annotateRewriteTarget).
		WithSpec((*networkv1apply.IngressSpecApplyConfiguration)(ssanginx.Spec.IngressSpec).
			WithIngressClassName(constants.IngressClassName))

	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: ssanginx.Spec.IngressName}, &ingress); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
		if !errors.IsNotFound(err) {
			return err
		}
	}

	if ssanginx.Spec.IngressSecureEnabled {
		// Re-create Secret if 'spec.tls[].hosts[]' has changed
		if len(ingress.GetName()) > 0 {
			if err := r.List(ctx, &secrets, client.InNamespace(ssanginx.GetNamespace()),
				client.MatchingFields(map[string]string{constants.IndexOwnerKey: ssanginx.GetName()})); err != nil {
				return err
			}

			ih := ingress.Spec.TLS[0].Hosts[0]
			sh := *ssanginx.Spec.IngressSpec.Rules[0].Host
			if ih != sh {
				for _, secret := range secrets.Items {
					if err := r.Delete(ctx, &secret); err != nil {
						return err
					}
					log.Info(fmt.Sprintf("delete Secret resource: %s", secret.GetName()))
					r.Recorder.Eventf(&secret, corev1.EventTypeNormal, "Deleted", "Deleted Secret %q", secret.GetName())
				}
			}
		}

		if err := r.applyIngressSecret(ctx, constants.FieldManager, log, ssanginx); err != nil {
			log.Error(err, "Unable create Ingress Secret")
			return err
		}

		if err := r.applyClientSecret(ctx, constants.FieldManager, log, ssanginx); err != nil {
			log.Error(err, "Unable create Client Secret")
			return err
		}

		nextIngressApplyConfig.
			WithAnnotations(annotateVerifyClient).
			WithAnnotations(annotateTlsSecret).
			Spec.
			WithTLS(networkv1apply.IngressTLS().
				WithHosts(*ssanginx.Spec.IngressSpec.Rules[0].Host).
				WithSecretName(constants.IngressSecretName))
	}

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextIngressApplyConfig.WithOwnerReferences(owner)

	// Difference Check at Client-Side
	currIngressApplyConfig, err := networkv1apply.ExtractIngress(&ingress, fieldMgr)
	if err != nil {
		return err
	}
	if equality.Semantic.DeepEqual(currIngressApplyConfig, nextIngressApplyConfig) {
		return nil
	}

	applied, err := ingressClient.Apply(ctx, nextIngressApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Ingress Applied: %s", applied.GetName()))

	return nil
}

func (r *SSANginxReconciler) applyIngressSecret(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		secret       corev1.Secret
		secretClient = kclientset.CoreV1().Secrets(constants.Namespace)
	)

	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: constants.IngressSecretName}, &secret); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
		if !errors.IsNotFound(err) {
			return err
		}
	}

	if len(secret.GetName()) > 0 {
		return nil
	}

	caCrt, _, err := pki.CreateCaCrt()
	if err != nil {
		log.Error(err, "Unable create CA Certificates")
		return err
	}

	svrCrt, svrKey, err := pki.CreateSvrCrt(ssanginx)
	if err != nil {
		log.Error(err, "Unable create Server Certificates")
		return err
	}

	secData := map[string][]byte{
		"tls.crt": svrCrt,
		"tls.key": svrKey,
		"ca.crt":  caCrt,
	}

	nextIngressSecretApplyConfig := corev1apply.Secret(constants.IngressSecretName, constants.Namespace).
		WithData(secData)

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextIngressSecretApplyConfig.WithOwnerReferences(owner)

	applied, err := secretClient.Apply(ctx, nextIngressSecretApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Server Certificates Secret Applied: %s", applied.GetName()))

	return nil
}

func (r *SSANginxReconciler) applyClientSecret(ctx context.Context, fieldMgr string, log logr.Logger, ssanginx ssanginxv1.SSANginx) error {
	var (
		secret       corev1.Secret
		secretClient = kclientset.CoreV1().Secrets(constants.Namespace)
	)

	if err := r.Get(ctx, client.ObjectKey{Namespace: constants.Namespace, Name: constants.ClientSecretName}, &secret); err != nil {
		// If the resource does not exist, create it.
		// Therefore, Not Found errors are ignored.
		if !errors.IsNotFound(err) {
			return err
		}
	}

	if len(secret.GetName()) > 0 {
		return nil
	}

	cliCrt, cliKey, err := pki.CreateClientCrt()
	if err != nil {
		log.Error(err, "Unable create Client Certificates")
		return err
	}

	secData := map[string][]byte{
		"client.crt": cliCrt,
		"client.key": cliKey,
	}

	nextClientSecretApplyConfig := corev1apply.Secret(constants.ClientSecretName, constants.Namespace).
		WithData(secData)

	owner, err := createOwnerReferences(log, ssanginx, r.Scheme)
	if err != nil {
		log.Error(err, "Unable create OwnerReference")
		return err
	}
	nextClientSecretApplyConfig.WithOwnerReferences(owner)

	applied, err := secretClient.Apply(ctx, nextClientSecretApplyConfig, metav1.ApplyOptions{
		FieldManager: fieldMgr,
		Force:        true,
	})
	if err != nil {
		log.Error(err, "unable to apply")
		return err
	}

	log.Info(fmt.Sprintf("Nginx Client Certificates Secret Applied: %s", applied.GetName()))

	return nil
}

//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete

// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *SSANginxReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var (
		log      = r.Log.WithValues("ssanginx", req.NamespacedName)
		ssanginx ssanginxv1.SSANginx
	)

	if err := r.Get(ctx, req.NamespacedName, &ssanginx); err != nil {
		log.Error(err, "unable to fetch CR SSANginx")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Create Configmap
	// Generate default.conf and index.html
	if err := r.applyConfigMap(ctx, constants.FieldManager, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	// Create Deployment
	if err := r.applyDeployment(ctx, constants.FieldManager, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	// Create Service
	if err := r.applyService(ctx, constants.FieldManager, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	// Create Ingress
	if err := r.applyIngress(ctx, constants.FieldManager, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.deleteOwnedResources(ctx, log, ssanginx); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SSANginxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	var (
		apiGVStr = ssanginxv1.GroupVersion.String()
		ctx      = context.Background()
	)

	// add IndexOwnerKey index to configmap object which SSANginx resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.ConfigMap{}, constants.IndexOwnerKey, func(obj client.Object) []string {
		// grab the configmap object, extract the owner...
		configMap := obj.(*corev1.ConfigMap)
		owner := metav1.GetControllerOf(configMap)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != constants.CrKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	// add IndexOwnerKey index to deployment object which SSANginx resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &appsv1.Deployment{}, constants.IndexOwnerKey, func(obj client.Object) []string {
		// grab the deployment object, extract the owner...
		deployment := obj.(*appsv1.Deployment)
		owner := metav1.GetControllerOf(deployment)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != constants.CrKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	// add IndexOwnerKey index to service object which SSANginx resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Service{}, constants.IndexOwnerKey, func(obj client.Object) []string {
		// grab the service object, extract the owner...
		service := obj.(*corev1.Service)
		owner := metav1.GetControllerOf(service)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != constants.CrKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	// add IndexOwnerKey index to ingress object which SSANginx resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &networkv1.Ingress{}, constants.IndexOwnerKey, func(obj client.Object) []string {
		// grab the ingress object, extract the owner...
		ingress := obj.(*networkv1.Ingress)
		owner := metav1.GetControllerOf(ingress)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != constants.CrKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	// add IndexOwnerKey index to secret object which SSANginx resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Secret{}, constants.IndexOwnerKey, func(obj client.Object) []string {
		// grab the secret object, extract the owner...
		secret := obj.(*corev1.Secret)
		owner := metav1.GetControllerOf(secret)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != apiGVStr || owner.Kind != constants.CrKind {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&ssanginxv1.SSANginx{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkv1.Ingress{}).
		Complete(r)
}
