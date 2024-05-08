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
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var mirrorlog = logf.Log.WithName("mirror-resource")

var kclient client.Client

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (r *Mirror) SetupWebhookWithManager(mgr ctrl.Manager) error {
	kclient = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-image-lin2ur-cn-v1-mirror,mutating=false,failurePolicy=fail,sideEffects=None,groups=image.lin2ur.cn,resources=mirrors,verbs=create;update,versions=v1,name=vmirror.kb.io,admissionReviewVersions=v1
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list

var _ webhook.Validator = &Mirror{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Mirror) ValidateCreate() (admission.Warnings, error) {
	return nil, r.validate()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Mirror) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (r *Mirror) validate() error {
	if r.Spec.DockerConfig != nil {
		secret := &corev1.Secret{}
		path := field.NewPath("spec").Key("dockerConfig")

		if err := kclient.Get(context.Background(), client.ObjectKey{
			Namespace: r.Namespace,
			Name:      r.Spec.DockerConfig.SecretName,
		}, secret); err != nil {
			return field.NotFound(
				path,
				r.Spec.DockerConfig.SecretName,
			)
		}

		if secret.Type != corev1.SecretTypeDockerConfigJson {
			return field.TypeInvalid(
				path,
				secret.Type,
				string("Secret type must be "+corev1.SecretTypeDockerConfigJson),
			)
		}
	}
	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Mirror) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}
