package controller

import (
	"context"
	"fmt"
	v12 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/imdario/mergo"
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
	"sync"
	"time"
)

const WebhookPathPrefix = "/mutate-pod/"

var webhookClientConfig admissionregistrationv1.WebhookClientConfig

func init() {
	caCert, err := os.ReadFile(getEnvOrDie("WEBHOOK_CA_CERT_FILE"))
	if err != nil {
		panic("failed to read CA cert file: " + err.Error())
	}

	webhookClientConfig = admissionregistrationv1.WebhookClientConfig{
		CABundle: caCert,
		Service: &admissionregistrationv1.ServiceReference{
			Name:      getEnvOrDie("WEBHOOK_SERVICE_NAME"),
			Namespace: getEnvOrDie("WEBHOOK_SERVICE_NAMESPACE"),
			Port:      ptr.To(int32(webhook.DefaultPort)),
		},
	}
}

func buildMutateHandler(decoder *admission.Decoder, rule imagev1.Rule) admission.HandlerFunc {
	return func(ctx context.Context, request admission.Request) (response admission.Response) {
		pod := &corev1.Pod{}
		if err := decoder.Decode(request, pod); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		logger := log.FromContext(ctx)

		var (
			patches []jsonpatch.JsonPatchOperation
			images  []string
		)

		processContainers := func(containers []corev1.Container, initContainers bool) bool {
			var containerPath string
			if initContainers {
				containerPath = "initContainers"
			} else {
				containerPath = "containers"
			}

			for i, container := range containers {
				hasDisallowedTag := slices.ContainsFunc(rule.Spec.DisallowedTags, func(s string) bool {
					return s == getImageTag(container.Image)
				})

				if hasDisallowedTag {
					response = admission.Denied(fmt.Sprintf(
						"[%s] tags is not allowed in %s: %s",
						strings.Join(rule.Spec.DisallowedTags, " "), containerPath, container.Name,
					))

					return false
				}

				image, isRewrite := rewriteImage(container.Image, rule.Spec.Rules)
				if !isRewrite {
					images = append(images, container.Image)
					continue
				}

				images = append(images, image)

				patches = append(patches, jsonpatch.JsonPatchOperation{
					Operation: "replace",
					Path:      fmt.Sprintf("/spec/%s/%d/image", containerPath, i),
					Value:     image,
				})

				logger.Info(
					"rewrite image",
					"container", containerPath+"/"+container.Name,
					"image", image,
				)
			}

			return true
		}

		if !processContainers(pod.Spec.InitContainers, true) ||
			!processContainers(pod.Spec.Containers, false) {
			return
		}

		if !rule.Spec.SetArchNodeAffinity {
			return admission.Patched("", patches...)
		}

		var wg sync.WaitGroup
		ch := make(chan []*v12.Platform, len(images))

		timeout, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		for _, image := range images {
			wg.Add(1)
			go func() {
				defer wg.Done()

				imagePlatform, err := getImagePlatform(timeout, image)
				if err != nil {
					logger.Error(err, "failed to get image platform", "image", image)
				}

				if len(imagePlatform) > 0 {
					ch <- imagePlatform
				}
			}()
		}

		wg.Wait()
		close(ch)

		if len(ch) == 0 {
			return admission.Patched("", patches...)
		}

		var platforms []*v12.Platform
		for result := range ch {
			platforms = intersection(platforms, result)
		}

		var arch []string
		for _, platform := range platforms {
			if platform.OS == "linux" {
				arch = append(arch, getArch(platform))
			}
		}

		affinity := pod.Spec.Affinity
		if affinity == nil {
			affinity = &corev1.Affinity{}
		}

		err := mergo.Merge(affinity, &corev1.Affinity{
			NodeAffinity: &corev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{},
				},
			},
		})

		if err != nil {
			logger.Error(err, "failed to merge affinity")
			return
		}

		nodeSelectorTerm := corev1.NodeSelectorTerm{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{
					Key:      "kubernetes.io/arch",
					Operator: corev1.NodeSelectorOpIn,
					Values:   arch,
				},
			},
		}

		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = append(
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms,
			nodeSelectorTerm,
		)

		patches = append(patches, jsonpatch.JsonPatchOperation{
			Operation: "replace",
			Path:      "/spec/affinity",
			Value:     affinity,
		})

		return admission.Patched("", patches...)
	}
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

func getEnvOrDie(name string) string {
	value := os.Getenv(name)
	if value == "" {
		panic("missing required environment variable " + name)
	}
	return value
}

func deref[T any](v *T) *T {
	if v != nil {
		return v
	}
	return new(T)
}

func intersection(a, b []*v12.Platform) []*v12.Platform {
	if len(a) == 0 {
		return b
	}

	m := make(map[string]bool)
	result := make([]*v12.Platform, 0)

	for _, item := range a {
		m[item.String()] = true
	}

	for _, item := range b {
		if _, ok := m[item.String()]; ok {
			delete(m, item.String())
			result = append(result, item)
		}
	}

	return result
}

func getArch(platform *v12.Platform) string {
	switch true {
	case platform.Architecture == "arm" && platform.Variant == "v7":
		return "arm64"
	default:
		return platform.Architecture
	}
}