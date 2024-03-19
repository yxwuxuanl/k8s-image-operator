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

package controller

import (
	"context"
	"errors"
	"flag"
	imagev1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"github.com/yxwuxuanl/k8s-image-operator/internal/utils"
	v1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
	"sync"
)

var (
	namespace   = flag.String("namespace", os.Getenv("NAMESPACE"), "")
	serviceName = flag.String("service", "", "")
)

// RuleReconciler reconciles a Rule object
type RuleReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	decoder *admission.Decoder

	rules map[string]imagev1.RuleSpec
	mux   sync.RWMutex
}

//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules,verbs=get;list;watch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/finalizers,verbs=update
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=create;list;watch
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;delete;patch,resourceNames=image-operator

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Rule object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.0/pkg/reconcile
func (r *RuleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconcile", "obj", req.Name)

	rule := new(imagev1.Rule)

	if err := r.Get(ctx, req.NamespacedName, rule); err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.delete(ctx, req.Name); err != nil {
				logger.Error(err, "failed to delete rule")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	if err := r.set(ctx, rule.Name, rule.Spec); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RuleReconciler) SetupWithManager(mgr ctrl.Manager, caCertFile string) error {
	if *namespace == "" {
		return errors.New("`--namespace` is required")
	}

	if *serviceName == "" {
		return errors.New("`--service` is required")
	}

	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return err
	}

	webhookClientConfig = v1.WebhookClientConfig{
		Service: &v1.ServiceReference{
			Namespace: *namespace,
			Name:      *serviceName,
			Port: utils.ToPtr(int32(webhook.DefaultPort)),
		},
		CABundle: caCert,
	}

	r.decoder = admission.NewDecoder(mgr.GetScheme())
	r.rules = make(map[string]imagev1.RuleSpec)

	mgr.GetWebhookServer().Register("/mutate-pod/", &webhook.Admission{
		Handler: r,
		WithContextFunc: func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(
				ctx, ruleNameCtxKey,
				strings.TrimPrefix(r.URL.Path, "/mutate-pod/"),
			)
		},
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&imagev1.Rule{}).
		Complete(r)
}
