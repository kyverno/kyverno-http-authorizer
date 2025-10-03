package authz

import (
	"context"
	"net"

	"github.com/kyverno/kyverno-http-authorizer/pkg/server"
	"github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func NewServer(network, addr string, vpolServer v1alpha1.ValidatingPolicyServiceServer) server.ServerFunc {
	return func(ctx context.Context) error {
		// create a server
		s := grpc.NewServer()
		// register our authorization service
		v1alpha1.RegisterValidatingPolicyServiceServer(s, vpolServer)
		reflection.Register(s)
		// create a listener
		l, err := net.Listen(network, addr)
		if err != nil {
			return err
		}
		// run server
		return server.RunGrpc(ctx, s, l)
	}
}
