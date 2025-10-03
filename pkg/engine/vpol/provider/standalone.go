package provider

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream"
	"github.com/kyverno/kyverno/api/policies.kyverno.io/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type standaloneReconciler struct {
	client       client.Client
	compiler     compiler.Compiler
	lock         *sync.Mutex
	policies     map[string]engine.CompiledPolicy
	sortPolicies func() []engine.CompiledPolicy
}

func NewStandaloneReconciler(client client.Client, compiler compiler.Compiler) *standaloneReconciler {
	return &standaloneReconciler{
		client:   client,
		compiler: compiler,
		lock:     &sync.Mutex{},
		policies: map[string]engine.CompiledPolicy{},
		sortPolicies: func() []engine.CompiledPolicy {
			return nil
		},
	}
}

func (r *standaloneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var policy v1alpha1.ValidatingPolicy
	// Reset the sorted func on every reconcile so the policies get resorted in next call
	resetSortPolicies := func() {
		r.sortPolicies = sync.OnceValue(func() []engine.CompiledPolicy {
			r.lock.Lock()
			defer r.lock.Unlock()
			return stream.MapToSortedSlice(r.policies)
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
	compiled, errs := r.compiler.Compile(&policy)
	if len(errs) > 0 {
		fmt.Println(errs)
		// No need to retry it
		return ctrl.Result{}, nil
	}
	r.lock.Lock()
	defer r.lock.Unlock()
	r.policies[req.String()] = compiled
	resetSortPolicies()
	return ctrl.Result{}, nil
}

func (r *standaloneReconciler) CompiledPolicies(ctx context.Context) ([]engine.CompiledPolicy, error) {
	return slices.Clone(r.sortPolicies()), nil
}
