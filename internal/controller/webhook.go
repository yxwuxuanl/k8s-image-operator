package controller

import (
	"encoding/json"
	"flag"
	"fmt"
	apiv1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"io"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"log"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"slices"
	"strings"
	"sync"
)

var (
	webhookAddr = flag.String("webhook-addr", ":443", "")
	tlsCertFile = flag.String("tls.cert-file", "./tls/cert", "")
	tlsKeyFile  = flag.String("tls.key-file", "./tls/key", "")
)

var (
	handlers    = make(map[string]http.HandlerFunc)
	noopHandler = createWebhookHandler("", apiv1.RuleSpec{})
	mux         sync.RWMutex
)

func StartWebhookServer() {
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		getWebhookHandler(name).ServeHTTP(rw, r)
	})

	ctrl.Log.Info("wehbook server listening at " + *webhookAddr)

	err := (&http.Server{Addr: *webhookAddr, Handler: handler}).ListenAndServeTLS(
		*tlsCertFile,
		*tlsKeyFile,
	)

	if err != nil {
		log.Fatal(err.Error())
	}
}

func getWebhookHandler(name string) http.HandlerFunc {
	mux.RLock()
	defer mux.RUnlock()

	if handler, ok := handlers[name]; ok {
		return handler
	}

	return noopHandler
}

func setWebhookHandler(name string, handler http.HandlerFunc) {
	mux.Lock()
	defer mux.Unlock()

	if handler != nil {
		handlers[name] = handler
	} else {
		delete(handlers, name)
	}
}

func createWebhookHandler(name string, spec apiv1.RuleSpec) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		admissionReview, pod, err := decodeAdmissionReview(r.Body)
		if err != nil {
			ctrl.Log.Error(err, "failed to decode admission review", "request", r.RequestURI)
			http.Error(rw, err.Error(), http.StatusBadRequest)
			return
		}

		admissionResponse := &admissionv1.AdmissionResponse{
			Allowed: true,
			UID:     admissionReview.Request.UID,
		}

		defer func() {
			admissionReview.Request = nil
			admissionReview.Response = admissionResponse

			json.NewEncoder(rw).Encode(admissionReview)
		}()

		if len(spec.DisallowedTags) == 0 && len(spec.Rules) == 0 {
			return
		}

		var patches []map[string]string

		processContainers := func(containers []v1.Container, initContainers bool) {
			var containerPath string
			if initContainers {
				containerPath = "initContainers"
			} else {
				containerPath = "containers"
			}

			for i, container := range containers {
				hasDisallowedTag := slices.ContainsFunc(spec.DisallowedTags, func(s string) bool {
					return s == getImageTag(container.Image)
				})

				if hasDisallowedTag {
					admissionResponse.Allowed = false
					admissionResponse.Result = &metav1.Status{
						Reason: "ImageTagNotAllowed",
						Message: fmt.Sprintf(
							"[%s] tags is not allowed in %s: %s",
							strings.Join(spec.DisallowedTags, " "), containerPath, container.Name,
						),
					}

					ctrl.Log.Info(admissionResponse.Result.Message,
						"pod", pod.Name+"/"+pod.Namespace,
						"rule", name,
					)
					return
				}

				image, isRewrite := rewriteImage(container.Image, spec.Rules)
				if !isRewrite {
					continue
				}

				patches = append(patches, map[string]string{
					"op":    "replace",
					"path":  fmt.Sprintf("/spec/%s/%d/image", containerPath, i),
					"value": image,
				})

				ctrl.Log.Info(
					"rewrite image",
					"pod", pod.Name+"/"+pod.Namespace,
					"rule", name,
					"container", containerPath+"/"+container.Name,
					"source", container.Image,
					"rewrite", image,
				)
			}
		}

		processContainers(pod.Spec.InitContainers, true)
		processContainers(pod.Spec.Containers, false)

		if admissionResponse.Allowed && len(patches) > 0 {
			jsonPatch := admissionv1.PatchTypeJSONPatch
			admissionResponse.PatchType = &jsonPatch
			admissionResponse.Patch, _ = json.Marshal(patches)
		}
	}
}

func decodeAdmissionReview(r io.ReadCloser) (*admissionv1.AdmissionReview, *v1.Pod, error) {
	defer r.Close()

	admissionReview := new(admissionv1.AdmissionReview)
	if err := json.NewDecoder(r).Decode(admissionReview); err != nil {
		return nil, nil, fmt.Errorf("failed to decode admission review: %w", err)
	}

	pod := new(v1.Pod)
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, pod); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal request object: %w", err)
	}

	return admissionReview, pod, nil
}
