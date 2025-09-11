package authz

import (
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"google.golang.org/grpc"
)

type service struct {
	v1alpha1.UnimplementedValidatingPolicyServiceServer
	provider engine.Provider
}

func (s *service) ValidatePoliciesStream(grpc.BidiStreamingServer[v1alpha1.ValidatingPolicyRequest, v1alpha1.ValidatingPolicyResponse]) error {
	return nil
}
