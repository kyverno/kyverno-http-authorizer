package authzserver

import (
	"context"

	"github.com/kyverno/kyverno-http-authorizer/pkg/authz"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/httpauth"
	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/listener"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
)

func Command() *cobra.Command {
	var probesAddress string
	var metricsAddress string
	var grpcAddress string
	var grpcNetwork string
	var httpAuthAddress string
	var kubeConfigOverrides clientcmd.ConfigOverrides
	var externalPolicySources []string
	var controlPlaneAddr string
	command := &cobra.Command{
		Use:   "authz-server",
		Short: "Start the Kyverno Authz Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// setup signals aware context
			return signals.Do(context.Background(), func(ctx context.Context) error {
				// track errors
				var httpErr, grpcErr, mgrErr, httpAuthErr error
				err := func(ctx context.Context) error {
					logger := logrus.New()
					// create a cancellable context
					ctx, cancel := context.WithCancel(ctx)
					// cancel context at the end
					defer cancel()
					// create a wait group
					var group wait.Group
					// wait all tasks in the group are over
					defer group.Wait()

					vpolCompiler := vpolcompiler.NewCompiler()

					provider := listener.NewPolicyListener(ctx, cancel, controlPlaneAddr, vpolCompiler, logger)
					// create http and grpc servers
					http := probes.NewServer(probesAddress)
					grpc := authz.NewServer(grpcNetwork, grpcAddress, provider)

					httpAuth := httpauth.NewServer(httpAuthAddress, provider)
					// run servers
					group.StartWithContext(ctx, func(ctx context.Context) {
						// cancel context at the end
						defer cancel()
						httpErr = http.Run(ctx)
					})
					group.StartWithContext(ctx, func(ctx context.Context) {
						// cancel context at the end
						defer cancel()
						httpAuthErr = httpAuth.Run(ctx)
					})
					group.StartWithContext(ctx, func(ctx context.Context) {
						// cancel context at the end
						defer cancel()
						grpcErr = grpc.Run(ctx)
					})
					return nil
				}(ctx)
				return multierr.Combine(err, httpErr, grpcErr, mgrErr, httpAuthErr)
			})
		},
	}
	command.Flags().StringVar(&probesAddress, "probes-address", ":9080", "Address to listen on for health checks")
	command.Flags().StringVar(&grpcAddress, "grpc-address", ":9081", "Address to listen on")
	command.Flags().StringVar(&httpAuthAddress, "http-auth-server-address", ":9083", "Address to serve the http authorization server on")
	command.Flags().StringVar(&metricsAddress, "metrics-address", ":9082", "Address to listen on for metrics")
	command.Flags().StringArrayVar(&externalPolicySources, "external-policy-source", nil, "External policy sources")
	clientcmd.BindOverrideFlags(&kubeConfigOverrides, command.Flags(), clientcmd.RecommendedConfigOverrideFlags("kube-"))
	return command
}
