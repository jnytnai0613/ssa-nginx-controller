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

package v1

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var (
	newScheme   = runtime.NewScheme()
	ssanginxlog = logf.Log.WithName("ssanginx-resource")
)

func init() {
	utilruntime.Must(AddToScheme(newScheme))
}

func (r *SSANginx) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

/*
// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
//+kubebuilder:webhook:path=/mutate-ssanginx-jnytnai0613-github-io-v1-ssanginx,mutating=true,failurePolicy=fail,sideEffects=None,groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes,verbs=create;update,versions=v1,name=mssanginx.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &SSANginx{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *SSANginx) Default() {
	ssanginxlog.Info("default", "name", r.Name)

	// TODO(user): fill in your defaulting logic.
}
*/

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-ssanginx-jnytnai0613-github-io-v1-ssanginx,mutating=false,failurePolicy=fail,sideEffects=None,groups=ssanginx.jnytnai0613.github.io,resources=ssanginxes,verbs=create;update,versions=v1,name=vssanginx.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &SSANginx{}

func (r *SSANginx) validateIngressServiceName() *field.Error {
	if r.Spec.ServiceName != *r.Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service.Name {

		return field.Invalid(field.NewPath("Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service").Child("Name"),
			r.Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service.Name, "Must match service name.")

	}

	return nil
}

func (r *SSANginx) validateIngressServicePort() *field.Error {
	if *r.Spec.ServiceSpec.Ports[0].Port != *r.Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number {

		return field.Invalid(field.NewPath("r.Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service.Port").Child("Number"),
			r.Spec.IngressSpec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number, "Must match service port number.")

	}

	return nil
}

//https://github.com/govargo/foo-controller-kubebuilder/blob/admission_webhook/api/v1alpha1/foo_webhook.go
func (r *SSANginx) validateIngress() error {
	var allErrs field.ErrorList
	gvk, err := apiutil.GVKForObject(r, newScheme)
	if err != nil {
		ssanginxlog.Error(err, "Unable get GVK")
		return err
	}

	if err := r.validateIngressServiceName(); err != nil {
		allErrs = append(allErrs, err)
	}

	if err := r.validateIngressServicePort(); err != nil {
		allErrs = append(allErrs, err)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: gvk.Group, Kind: gvk.Kind}, r.Name, allErrs)
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *SSANginx) ValidateCreate() error {
	ssanginxlog.Info("validate create", "name", r.Name)

	return r.validateIngress()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *SSANginx) ValidateUpdate(old runtime.Object) error {
	ssanginxlog.Info("validate update", "name", r.Name)

	return r.validateIngress()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *SSANginx) ValidateDelete() error {
	ssanginxlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil
}
