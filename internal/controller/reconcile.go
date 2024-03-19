package controller

import (
	"context"
	"encoding/json"
	"fmt"
	apiv1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"github.com/yxwuxuanl/k8s-image-operator/internal/utils"
	"gomodules.xyz/jsonpatch/v2"
	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"slices"
)

const MutatingWebhookConfigurationName = "image-operator"

var (
	webhookClientConfig             v1.WebhookClientConfig
	mutatingWebhookConfigurationKey = client.ObjectKey{Name: MutatingWebhookConfigurationName}
)

func (r *RuleReconciler) set(ctx context.Context, name string, spec apiv1.RuleSpec) error {
	r.mux.Lock()
	r.rules[name] = spec
	r.mux.Unlock()

	mutatingWebhookConfiguration := &v1.MutatingWebhookConfiguration{}

	if err := r.Get(ctx, mutatingWebhookConfigurationKey, mutatingWebhookConfiguration); err != nil {
		if errors.IsNotFound(err) {
			mutatingWebhookConfiguration := &v1.MutatingWebhookConfiguration{
				ObjectMeta: metav1.ObjectMeta{
					Name: MutatingWebhookConfigurationName,
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "image-operator",
					},
				},
				Webhooks: []v1.MutatingWebhook{createMutatingWebhook(name, spec)},
			}

			return r.Create(ctx, mutatingWebhookConfiguration)
		}

		return err
	}

	var patches []jsonpatch.JsonPatchOperation

	index := slices.IndexFunc(mutatingWebhookConfiguration.Webhooks, func(webhook v1.MutatingWebhook) bool {
		return webhook.Name == getMutatingWebhookName(name)
	})

	if index >= 0 {
		patches = append(patches, jsonpatch.JsonPatchOperation{
			Operation: "replace",
			Path:      fmt.Sprintf("/webhooks/%d", index),
			Value:     createMutatingWebhook(name, spec),
		})
	} else {
		patches = append(patches, jsonpatch.JsonPatchOperation{
			Operation: "add",
			Path:      "/webhooks/-",
			Value:     createMutatingWebhook(name, spec),
		})
	}

	jsonPatch, _ := json.Marshal(patches)

	return r.Patch(ctx, mutatingWebhookConfiguration, client.RawPatch(types.JSONPatchType, jsonPatch))
}

func (r *RuleReconciler) delete(ctx context.Context, name string) error {
	defer func() {
		r.mux.Lock()
		defer r.mux.Unlock()
		delete(r.rules, name)
	}()

	mutatingWebhookConfiguration := &v1.MutatingWebhookConfiguration{}

	if err := r.Get(ctx, mutatingWebhookConfigurationKey, mutatingWebhookConfiguration); err != nil {
		return client.IgnoreNotFound(err)
	}

	index := slices.IndexFunc(mutatingWebhookConfiguration.Webhooks, func(webhook v1.MutatingWebhook) bool {
		return webhook.Name == getMutatingWebhookName(name)
	})

	if index < 0 {
		return nil
	}

	if len(mutatingWebhookConfiguration.Webhooks) == 1 {
		return r.Delete(ctx, mutatingWebhookConfiguration)
	}

	patches := fmt.Sprintf(`[{"op":"remove","path":"/webhooks/%d"}]`, index)

	return r.Patch(
		ctx,
		mutatingWebhookConfiguration,
		client.RawPatch(types.JSONPatchType, []byte(patches)),
	)
}

func createMutatingWebhook(name string, spec apiv1.RuleSpec) v1.MutatingWebhook {
	return v1.MutatingWebhook{
		Name:                    getMutatingWebhookName(name),
		ClientConfig:            createWebhookClientConfig(name),
		AdmissionReviewVersions: []string{"v1"},
		Rules: []v1.RuleWithOperations{
			{
				Operations: []v1.OperationType{v1.Create},
				Rule: v1.Rule{
					Resources:   []string{"pods"},
					APIGroups:   []string{""},
					APIVersions: []string{"*"},
				},
			},
		},
		NamespaceSelector: spec.NamespaceSelector,
		ObjectSelector:    spec.PodSelector,
		FailurePolicy:     spec.FailurePolicy,
		TimeoutSeconds:    utils.ToPtr(int32(5)),
		SideEffects:       utils.ToPtr(v1.SideEffectClassNone),
	}
}

func createWebhookClientConfig(name string) v1.WebhookClientConfig {
	conf := webhookClientConfig

	name = "/mutate-pod/" + name
	conf.Service.Path = &name

	return conf
}

func getMutatingWebhookName(name string) string {
	return name + ".image-operator.io"
}
