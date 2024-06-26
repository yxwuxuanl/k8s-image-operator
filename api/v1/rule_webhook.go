/*
Copyright 2024.

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
	"errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"regexp"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var rulelog = logf.Log.WithName("rule-resource")

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Rule) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-image-lin2ur-cn-v1-rule,mutating=true,failurePolicy=fail,sideEffects=None,groups=image.lin2ur.cn,resources=rules,verbs=create;update,versions=v1,name=mrule.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &Rule{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Rule) Default() {
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-image-lin2ur-cn-v1-rule,mutating=false,failurePolicy=fail,sideEffects=None,groups=image.lin2ur.cn,resources=rules,verbs=create;update,versions=v1,name=vrule.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Rule{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateCreate() (admission.Warnings, error) {
	return nil, r.validate()
}

func (r *Rule) validate() error {
	if len(r.Spec.Rewrite) == 0 && len(r.Spec.DisallowedTags) == 0 {
		return errors.New("`rewrite` and `disallowedTags` cannot both be empty")
	}

	for i, rule := range r.Spec.Rewrite {
		if rule.Regex != "" {
			if _, err := regexp.Compile(rule.Regex); err != nil {
				return field.Invalid(
					field.NewPath("spec").Child("rewrite").Index(i).Key("regex"),
					rule.Regex,
					err.Error(),
				)
			}
		}
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return nil, r.validate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
