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
	"fmt"
	"github.com/yxwuxuanl/k8s-image-operator/internal/utils"
	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	if r.Spec.FailurePolicy == nil {
		r.Spec.FailurePolicy = utils.ToPtr(v1.Ignore)
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-image-lin2ur-cn-v1-rule,mutating=false,failurePolicy=fail,sideEffects=None,groups=image.lin2ur.cn,resources=rules,verbs=create;update,versions=v1,name=vrule.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Rule{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateCreate() (admission.Warnings, error) {
	if len(r.Spec.Rules) == 0 && len(r.Spec.DisallowedTags) == 0 {
		return nil, errors.New("`rules` and `disallowedTags` cannot both be empty")
	}

	for _, rule := range r.Spec.Rules {
		if rule.Regex != "" {
			if _, err := regexp.Compile(rule.Regex); err != nil {
				return nil, fmt.Errorf(
					"failed to compile regexp: %s: %w",
					rule.Regex, err,
				)
			}
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return r.ValidateCreate()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Rule) ValidateDelete() (admission.Warnings, error) {
	rulelog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
