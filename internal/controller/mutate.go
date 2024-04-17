package controller

import (
	"context"
	"fmt"
	imagev1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"gomodules.xyz/jsonpatch/v2"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"net/http"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"slices"
	"strings"
)

const WebhookPathPrefix = "/mutate-pod/"

var webhookClientConfig admissionregistrationv1.WebhookClientConfig

func initWebhookClientConfig() error {
	caCert, err := os.ReadFile(getEnvOrDie("WEBHOOK_CA_CERT_FILE"))
	if err != nil {
		return fmt.Errorf("failed to read CA cert file: %w", err)
	}

	webhookClientConfig = admissionregistrationv1.WebhookClientConfig{
		CABundle: caCert,
		Service: &admissionregistrationv1.ServiceReference{
			Name:      getEnvOrDie("WEBHOOK_SERVICE_NAME"),
			Namespace: getEnvOrDie("WEBHOOK_SERVICE_NAMESPACE"),
			Port:      ptr.To(int32(webhook.DefaultPort)),
		},
	}

	return nil
}

func buildMutateHandler(decoder *admission.Decoder, rule imagev1.Rule) admission.HandlerFunc {
	return func(ctx context.Context, request admission.Request) (response admission.Response) {
		pod := &corev1.Pod{}
		if err := decoder.Decode(request, pod); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		var patches []jsonpatch.JsonPatchOperation

		if v, err := mutateContainers(rule, pod.Spec.InitContainers, true); err == nil {
			patches = append(patches, v...)
		} else {
			return admission.Denied(err.Error())
		}

		if v, err := mutateContainers(rule, pod.Spec.Containers, false); err == nil {
			patches = append(patches, v...)
		} else {
			return admission.Denied(err.Error())
		}

		return admission.Patched("", patches...)
	}
}

func mutateContainers(rule imagev1.Rule, containers []corev1.Container, isInitContainers bool) (patches []jsonpatch.JsonPatchOperation, err error) {
	var containerPath string
	if isInitContainers {
		containerPath = "initContainers"
	} else {
		containerPath = "containers"
	}

	for i, container := range containers {
		hasDisallowedTag := slices.ContainsFunc(rule.Spec.DisallowedTags, func(s string) bool {
			return s == getImageTag(container.Image)
		})

		if hasDisallowedTag {
			return nil, fmt.Errorf(
				"[%s] tags is not allowed in %s: %s",
				strings.Join(rule.Spec.DisallowedTags, " "), containerPath, container.Name,
			)
		}

		if image, isRewrite := rewriteImage(container.Image, rule.Spec.Rules); isRewrite {
			patches = append(patches, jsonpatch.NewOperation(
				"replace",
				fmt.Sprintf("/spec/%s/%d/image", containerPath, i),
				image,
			))
		}
	}

	return patches, nil
}

func updateMutatingWebhookConfiguration(ctx context.Context, cli client.Client, rules []imagev1.Rule) error {
	mutatingWebhookConfiguration := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: v1.ObjectMeta{
			Name: "webhook." + imagev1.GroupVersion.Group,
		},
	}

	if len(rules) == 0 {
		if err := cli.Delete(ctx, mutatingWebhookConfiguration); err != nil {
			if !errors.IsNotFound(err) {
				log.FromContext(ctx).Error(err, "failed to delete mutating webhook configuration")
				return err
			}
		}
		return nil
	}

	var webhooks []admissionregistrationv1.MutatingWebhook

	for _, rule := range rules {
		clientConfig := webhookClientConfig.DeepCopy()
		clientConfig.Service.Path = ptr.To(WebhookPathPrefix + rule.Name)

		webhooks = append(webhooks, admissionregistrationv1.MutatingWebhook{
			Name:              rule.Name + "." + imagev1.GroupVersion.Group,
			ClientConfig:      *clientConfig,
			ObjectSelector:    rule.Spec.PodSelector,
			NamespaceSelector: rule.Spec.NamespaceSelector,
			FailurePolicy:     ptr.To(admissionregistrationv1.FailurePolicyType(rule.Spec.FailurePolicy)),
			SideEffects:       ptr.To(admissionregistrationv1.SideEffectClassNone),
			AdmissionReviewVersions: []string{
				admissionregistrationv1.SchemeGroupVersion.Version,
			},
			Rules: []admissionregistrationv1.RuleWithOperations{
				{
					Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"pods"},
					},
				},
			},
		})
	}

	op, err := controllerutil.CreateOrUpdate(ctx, cli, mutatingWebhookConfiguration, func() error {
		mutatingWebhookConfiguration.Webhooks = webhooks
		return nil
	})

	if err != nil {
		log.FromContext(ctx).Error(err, "failed to create or update mutating webhook configuration")
		return err
	}

	log.FromContext(ctx).Info(string("mutating webhook configuration has been " + op))
	return nil
}
