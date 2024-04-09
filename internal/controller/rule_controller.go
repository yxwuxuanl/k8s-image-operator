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
	imagev1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"net/http"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"slices"
	"strings"
	"sync"
)

var ruleNameCtxKey = struct{}{}

// RuleReconciler reconciles a Rule object
type RuleReconciler struct {
	client.Client
	Scheme  *runtime.Scheme
	decoder *admission.Decoder

	handlers sync.Map
}

//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules,verbs=get;list;watch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/finalizers,verbs=update
//+kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=create;list;watch;get;delete;patch

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

	var rules imagev1.RuleList
	if err := r.List(ctx, &rules); err != nil {
		return ctrl.Result{}, err
	}

	index := slices.IndexFunc(rules.Items, func(rule imagev1.Rule) bool {
		return rule.Name == req.Name
	})

	if index >= 0 {
		r.handlers.Store(req.Name, buildMutateHandler(r.decoder, rules.Items[index]))
	} else {
		r.handlers.Delete(req.Name)
	}

	if err := updateMutatingWebhookConfiguration(ctx, r.Client, rules.Items); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *RuleReconciler) Handle(ctx context.Context, request admission.Request) admission.Response {
	ruleName, ok := ctx.Value(ruleNameCtxKey).(string)
	if !ok {
		return admission.Errored(http.StatusBadRequest, errors.New("rule name not found"))
	}

	if v, ok := r.handlers.Load(ruleName); ok {
		return v.(admission.Handler).Handle(ctx, request)
	}

	return admission.Allowed("")
}

// SetupWithManager sets up the controller with the Manager.
func (r *RuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := initWebhookClientConfig(); err != nil {
		return err
	}

	r.decoder = admission.NewDecoder(mgr.GetScheme())

	mgr.GetWebhookServer().Register(WebhookPathPrefix, &webhook.Admission{
		Handler: r,
		WithContextFunc: func(ctx context.Context, r *http.Request) context.Context {
			return context.WithValue(
				ctx,
				ruleNameCtxKey,
				strings.TrimPrefix(r.URL.Path, WebhookPathPrefix),
			)
		},
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&imagev1.Rule{}).
		Complete(r)
}
