package authz

import (
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
)

type service struct {
	v1alpha1.UnimplementedValidatingPolicyServiceServer
	provider engine.Provider
}
