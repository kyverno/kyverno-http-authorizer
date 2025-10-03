package provider

import (
	"context"

	policyapi "github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/sender"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/kyverno/kyverno/api/policies.kyverno.io/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type meshReconciler struct {
	client    client.Client
	polSender *sender.PolicySender
}

func NewMeshReconciler(client client.Client, sender *sender.PolicySender) *meshReconciler {
	return &meshReconciler{
		client:    client,
		polSender: sender,
	}
}

func (r *meshReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policy v1alpha1.ValidatingPolicy
	err := r.client.Get(ctx, req.NamespacedName, &policy)
	if errors.IsNotFound(err) {
		r.polSender.DeletePolicy(req.Name)
		go r.polSender.SendPolicy(&protov1alpha1.ValidatingPolicy{
			Name:   req.Name,
			Delete: true,
		})
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if policy.Spec.EvaluationConfiguration.Mode != policyapi.EvaluationModeHTTP {
		return ctrl.Result{}, nil
	}
	protoPolicy := policyapi.ToProto(&policy)
	r.polSender.StorePolicy(protoPolicy)
	go r.polSender.SendPolicy(protoPolicy)
	return ctrl.Result{}, nil
}
