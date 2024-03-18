package controller

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	apiv1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	v1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"slices"
)

var (
	excludeOperatorNamespace = flag.Bool("exclude-operator-namespace", true, "")
)

const MutatingWebhookConfigurationName = "image-operator"

var webhookClientConfig v1.WebhookClientConfig

func (r *RuleReconciler) setRule(ctx context.Context, name string, spec apiv1.RuleSpec) error {
	mutatingWebhookConfiguration, err := r.clientset.
		AdmissionregistrationV1().
		MutatingWebhookConfigurations().
		Get(ctx, MutatingWebhookConfigurationName, metav1.GetOptions{})

	if err != nil {
		if !errors.IsNotFound(err) {
			ctrl.Log.Error(err, "setRule: failed to get mutatingWebhookConfiguration")
			return err
		}

		mutatingWebhookConfiguration = &v1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: MutatingWebhookConfigurationName,
				Labels: map[string]string{
					"app.kubernetes.io/managed-by": "image-operator",
				},
			},
			Webhooks: []v1.MutatingWebhook{createMutatingWebhook(name, spec)},
		}

		_, err := r.clientset.
			AdmissionregistrationV1().
			MutatingWebhookConfigurations().
			Create(ctx, mutatingWebhookConfiguration, metav1.CreateOptions{})

		if err != nil {
			ctrl.Log.Error(err, "setRule: failed to create mutatingWebhookConfiguration")
			return err
		}

		return nil
	}

	var patches []map[string]any

	index := slices.IndexFunc(mutatingWebhookConfiguration.Webhooks, func(webhook v1.MutatingWebhook) bool {
		return webhook.Name == getMutatingWebhookName(name)
	})

	if index >= 0 {
		patches = append(patches, map[string]any{
			"op":    "replace",
			"path":  fmt.Sprintf("/webhooks/%d", index),
			"value": createMutatingWebhook(name, spec),
		})
	} else {
		patches = append(patches, map[string]any{
			"op":    "add",
			"path":  "/webhooks/-",
			"value": createMutatingWebhook(name, spec),
		})
	}

	jsonPatch, _ := json.Marshal(patches)

	_, err = r.clientset.
		AdmissionregistrationV1().
		MutatingWebhookConfigurations().
		Patch(
			ctx,
			MutatingWebhookConfigurationName,
			types.JSONPatchType,
			jsonPatch,
			metav1.PatchOptions{},
		)

	if err != nil {
		ctrl.Log.Error(err, "setRule: failed to patch mutatingWebhookConfiguration", "rule", name)
		return err
	}

	return nil
}

func (r *RuleReconciler) deleteRule(ctx context.Context, name string) error {
	mutatingWebhookConfiguration, err := r.clientset.
		AdmissionregistrationV1().
		MutatingWebhookConfigurations().
		Get(ctx, MutatingWebhookConfigurationName, metav1.GetOptions{})

	if err != nil {
		ctrl.Log.Error(err, "deleteRule: failed to get mutatingWebhookConfiguration")
		if errors.IsNotFound(err) {
			return nil
		}

		return err
	}

	index := slices.IndexFunc(mutatingWebhookConfiguration.Webhooks, func(webhook v1.MutatingWebhook) bool {
		return webhook.Name == getMutatingWebhookName(name)
	})

	if index < 0 {
		return nil
	}

	if len(mutatingWebhookConfiguration.Webhooks) == 1 {
		err := r.clientset.
			AdmissionregistrationV1().
			MutatingWebhookConfigurations().
			Delete(ctx, MutatingWebhookConfigurationName, metav1.DeleteOptions{})

		if err != nil {
			ctrl.Log.Error(err, "deleteRule: failed to delete mutatingWebhookConfiguration")
			return err
		}

		return nil
	}

	patches := fmt.Sprintf(`[{"op":"remove","path":"/webhooks/%d"}]`, index)

	_, err = r.clientset.
		AdmissionregistrationV1().
		MutatingWebhookConfigurations().
		Patch(
			ctx,
			MutatingWebhookConfigurationName,
			types.JSONPatchType,
			[]byte(patches),
			metav1.PatchOptions{},
		)

	if err != nil {
		ctrl.Log.Error(err, "deleteRule: failed to patch mutatingWebhookConfiguration", "rule", name)
		return err
	}

	return nil
}

func createMutatingWebhook(name string, spec apiv1.RuleSpec) v1.MutatingWebhook {
	namespaceSelector := spec.NamespaceSelector

	if *excludeOperatorNamespace {
		if namespaceSelector == nil {
			namespaceSelector = new(metav1.LabelSelector)
		}

		namespaceSelector.MatchExpressions = append(namespaceSelector.MatchExpressions, metav1.LabelSelectorRequirement{
			Key:      "kubernetes.io/metadata.name",
			Operator: metav1.LabelSelectorOpNotIn,
			Values:   []string{*namespace},
		})
	}

	mutatingWebhook := v1.MutatingWebhook{
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
		NamespaceSelector: namespaceSelector,
		ObjectSelector:    spec.PodSelector,
	}

	mutatingWebhook.SideEffects = valueOrDefault(nil, v1.SideEffectClassNone)

	mutatingWebhook.FailurePolicy = valueOrDefault(spec.MutatingWebhookSpec.FailurePolicy, v1.Ignore)
	mutatingWebhook.TimeoutSeconds = valueOrDefault(spec.MutatingWebhookSpec.TimeoutSeconds, 5)

	return mutatingWebhook
}

func valueOrDefault[T any](n *T, defaultValue T) *T {
	if n != nil {
		return n
	}

	return &defaultValue
}

func createWebhookClientConfig(name string) v1.WebhookClientConfig {
	conf := webhookClientConfig

	name = "/" + name
	conf.Service.Path = &name

	return conf
}

func getMutatingWebhookName(name string) string {
	return name + ".image-operator.io"
}
