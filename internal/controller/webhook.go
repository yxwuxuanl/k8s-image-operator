package controller

import (
	"context"
	"fmt"
	v1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"gomodules.xyz/jsonpatch/v2"
	corev1 "k8s.io/api/core/v1"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"slices"
	"strings"
)

var ruleNameCtxKey = struct {
}{}

func (r *RuleReconciler) Handle(ctx context.Context, request admission.Request) admission.Response {
	pod := &corev1.Pod{}
	if err := r.decoder.Decode(request, pod); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	rs, ok := r.getRuleSpec(ctx)
	if !ok {
		return admission.Allowed("")
	}

	var patches []jsonpatch.JsonPatchOperation
	var denyResponse admission.Response

	processContainers := func(containers []corev1.Container, initContainers bool) bool {
		var containerPath string
		if initContainers {
			containerPath = "initContainers"
		} else {
			containerPath = "containers"
		}

		for i, container := range containers {
			hasDisallowedTag := slices.ContainsFunc(rs.DisallowedTags, func(s string) bool {
				return s == getImageTag(container.Image)
			})

			if hasDisallowedTag {
				denyResponse = admission.Denied(fmt.Sprintf(
					"[%s] tags is not allowed in %s: %s",
					strings.Join(rs.DisallowedTags, " "), containerPath, container.Name,
				))

				return false
			}

			image, isRewrite := rewriteImage(container.Image, rs.Rules)
			if !isRewrite {
				continue
			}

			patches = append(patches, jsonpatch.JsonPatchOperation{
				Operation: "replace",
				Path:      fmt.Sprintf("/spec/%s/%d/image", containerPath, i),
				Value:     image,
			})

			ctrl.Log.Info(
				"rewrite image",
				"pod", pod.Name+"/"+pod.Namespace,
				"container", containerPath+"/"+container.Name,
				"source", container.Image,
				"rewrite", image,
			)
		}

		return true
	}

	if !processContainers(pod.Spec.InitContainers, true) ||
		!processContainers(pod.Spec.Containers, false) {
		return denyResponse
	}

	return admission.Patched("", patches...)
}

func (r *RuleReconciler) getRuleSpec(ctx context.Context) (v1.RuleSpec, bool) {
	if v := ctx.Value(ruleNameCtxKey); v != nil {
		r.mux.RLock()
		defer r.mux.RUnlock()

		rs, ok := r.rules[v.(string)]
		return rs, ok
	}

	return v1.RuleSpec{}, false
}
