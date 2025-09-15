package authzserver

import (
	"context"
	"time"

	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/httpauth"
	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/listener"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/wait"
)

func Command() *cobra.Command {
	var probesAddress string
	var httpAuthAddress string
	var controlPlaneAddr string
	var controlPlaneReconnectWait int
	var controlPlaneMaxDialInterval int
	var clientAddr string
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

					// create another context for the control plane connection to avoid closing the auth server if the control plane exits
					grpcCtx, grpcCancel := context.WithCancel(context.Background())
					defer grpcCancel()

					// clientAddr := os.Getenv("POD_IP")
					// if clientAddr == "" {
					// 	return fmt.Errorf("can't start auth server, no POD_IP has been passed")
					// }

					vpolCompiler := vpolcompiler.NewCompiler()
					provider := listener.NewPolicyListener(ctx, cancel, controlPlaneAddr,
						clientAddr, vpolCompiler,
						logger, controlPlaneReconnectWait,
						controlPlaneMaxDialInterval)
					// create http and grpc servers
					http := probes.NewServer(probesAddress)
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
					group.StartWithContext(grpcCtx, func(ctx context.Context) {
						// cancel control plane grpc context at the end
						defer grpcCancel()
						if grpcErr = provider.Start(); grpcErr != nil {
							logger.Error("error connecting to the control plane, sleeping 10 seconds then retrying")
							time.Sleep(time.Second * 10)
						}
					})
					return nil
				}(ctx)
				return multierr.Combine(err, httpErr, grpcErr, mgrErr, httpAuthErr)
			})
		},
	}
	command.Flags().IntVar(&controlPlaneReconnectWait, "control-plane-reconnect-wait", 3, "Duration in seconds to wait before retrying connecting to the control plane")
	command.Flags().IntVar(&controlPlaneMaxDialInterval, "control-plane-max-dial-interval", 8, "Duration in seconds to wait before stopping attempts of sending a policy to a client")
	command.Flags().StringVar(&probesAddress, "probes-address", ":9088", "Address to listen on for health checks")
	command.Flags().StringVar(&httpAuthAddress, "http-auth-server-address", ":9083", "Address to serve the http authorization server on")
	command.Flags().StringVar(&controlPlaneAddr, "control-plane-address", "", "Control plane address")
	command.Flags().StringVar(&clientAddr, "client-address", "", "Client address")

	_ = command.MarkFlagRequired("control-plane-address")
	return command
}
