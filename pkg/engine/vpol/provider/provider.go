package provider

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/sender"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewKubeProvider(mgr ctrl.Manager, sender *sender.PolicySender) (*policyReconciler, error) {
	r := newPolicyReconciler(mgr.GetClient(), sender)
	if err := ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.ValidatingPolicy{}).Complete(r); err != nil {
		return nil, fmt.Errorf("failed to construct manager: %w", err)
	}
	return r, nil
}

type policyReconciler struct {
	client       client.Client
	lock         *sync.Mutex
	policies     map[string]engine.CompiledPolicy
	sortPolicies func() []engine.CompiledPolicy
	polSender    *sender.PolicySender
}

func newPolicyReconciler(client client.Client, sender *sender.PolicySender) *policyReconciler {
	return &policyReconciler{
		client:    client,
		lock:      &sync.Mutex{},
		polSender: sender,
		sortPolicies: func() []engine.CompiledPolicy {
			return nil
		},
	}
}

func mapToSortedSlice[K cmp.Ordered, V any](in map[K]V) []V {
	if in == nil {
		return nil
	}
	out := make([]V, 0, len(in))
	keys := maps.Keys(in)
	slices.Sort(keys)
	for _, key := range keys {
		out = append(out, in[key])
	}
	return out
}

func (r *policyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policy v1alpha1.ValidatingPolicy
	// Reset the sorted func on every reconcile so the policies get resorted in next call
	resetSortPolicies := func() {
		r.sortPolicies = sync.OnceValue(func() []engine.CompiledPolicy {
			r.lock.Lock()
			defer r.lock.Unlock()
			return mapToSortedSlice(r.policies)
		})
	}
	err := r.client.Get(ctx, req.NamespacedName, &policy)
	if errors.IsNotFound(err) {
		r.lock.Lock()
		defer r.lock.Unlock()
		defer resetSortPolicies()
		delete(r.policies, req.String())
		return ctrl.Result{}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	r.polSender.StorePolicy(&policy)
	go r.polSender.SendPolicy(&policy)
	return ctrl.Result{}, nil
}
