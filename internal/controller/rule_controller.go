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
	v1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"net"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"strconv"
)

var (
	namespace     = flag.String("namespace", os.Getenv("NAMESPACE"), "")
	serviceName   = flag.String("service", "", "")
	tlsCACertFile = flag.String("tls.ca-cert-file", "./tls/ca", "")
)

// RuleReconciler reconciles a Rule object
type RuleReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	clientset *kubernetes.Clientset
}

//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=rules/finalizers,verbs=update

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
	err := r.Get(ctx, req.NamespacedName, rule)

	if err != nil {
		if apierrors.IsNotFound(err) {
			if err := r.deleteRule(ctx, req.Name); err != nil {
				return ctrl.Result{}, err
			}

			setWebhookHandler(req.Name, nil)

			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	setWebhookHandler(req.Name, createWebhookHandler(req.Name, rule.Spec))

	if err = r.setRule(ctx, rule.Name, rule.Spec); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RuleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if *namespace == "" {
		return errors.New("`--namespace` is required")
	}

	if *serviceName == "" {
		return errors.New("`--service` is required")
	}

	var servicePort int32

	if _, v, err := net.SplitHostPort(*webhookAddr); err == nil {
		v, _ := strconv.ParseInt(v, 10, 32)
		servicePort = int32(v)
	} else {
		return err
	}

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.clientset = clientset

	caCert, err := os.ReadFile(*tlsCACertFile)
	if err != nil {
		return err
	}

	webhookClientConfig = v1.WebhookClientConfig{
		Service: &v1.ServiceReference{
			Namespace: *namespace,
			Name:      *serviceName,
			Port:      &servicePort,
		},
		CABundle: caCert,
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&imagev1.Rule{}).
		Complete(r)
}
