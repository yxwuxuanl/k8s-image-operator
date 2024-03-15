package controller

import (
	"encoding/json"
	"flag"
	"fmt"
	apiv1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	"log"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"strings"
	"sync"
)

var (
	webhookAddr = flag.String("webhook-addr", ":443", "")
	tlsCertFile = flag.String("tls.cert-file", "./tls/cert", "")
	tlsKeyFile  = flag.String("tls.key-file", "./tls/key", "")
)

var (
	handlers = make(map[string]http.HandlerFunc)
	mux      sync.RWMutex
)

func StartWebhookServer() {
	handler := http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")

		if handler := getWebhookHandler(name); handler != nil {
			handler(rw, r)
		} else {
			rw.WriteHeader(http.StatusNotFound)
		}
	})

	err := (&http.Server{Addr: *webhookAddr, Handler: handler}).ListenAndServeTLS(
		*tlsCertFile,
		*tlsKeyFile,
	)

	ctrl.Log.Info("wehbook server listening at " + *webhookAddr)

	if err != nil {
		log.Fatal(err.Error())
	}
}

func getWebhookHandler(name string) http.HandlerFunc {
	mux.RLock()
	defer mux.RUnlock()

	return handlers[name]
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

		var patches []map[string]string

		rewriteContainers := func(containers []v1.Container, initContainers bool) {
			for i, container := range containers {
				image, rewrite := rewriteImage(container.Image, spec.Rules)
				if !rewrite {
					continue
				}

				var containerPath string
				if initContainers {
					containerPath = "initContainers"
				} else {
					containerPath = "containers"
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

		rewriteContainers(pod.Spec.InitContainers, true)
		rewriteContainers(pod.Spec.Containers, false)

		if len(patches) > 0 {
			jsonPatch := admissionv1.PatchTypeJSONPatch
			admissionResponse.PatchType = &jsonPatch
			admissionResponse.Patch, _ = json.Marshal(patches)
		}
	}
}
