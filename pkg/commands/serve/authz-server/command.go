package authzserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/kyverno/kyverno-http-authorizer/pkg/cel/ctxprovider"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/httpauth"
	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/listener"
	"github.com/kyverno/kyverno/pkg/clients/dclient"
	dyn "github.com/kyverno/kyverno/pkg/clients/dynamic"
	meta "github.com/kyverno/kyverno/pkg/clients/metadata"

	nethttp "net/http"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func Command() *cobra.Command {
	var probesAddress string
	var httpAuthAddress string
	var controlPlaneAddr string
	var controlPlaneReconnectWait int
	var controlPlaneMaxDialInterval int
	// var clientAddr string
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

					clientAddr := os.Getenv("POD_IP")
					if clientAddr == "" {
						return fmt.Errorf("can't start auth server, no POD_IP has been passed")
					}

					cfg, err := rest.InClusterConfig()
					if err != nil {
						return err
					}

					// initialize kubernetes clients
					dclient, err := initK8sClients(ctx, cfg)
					if err != nil {
						return err
					}

					vpolCompiler := vpolcompiler.NewCompiler()
					provider := listener.NewPolicyListener(controlPlaneAddr,
						clientAddr, vpolCompiler,
						logger, controlPlaneReconnectWait,
						controlPlaneMaxDialInterval)

					// create http and grpc server
					http := probes.NewServer(probesAddress)
					// ammar: split the authorizer and pass it as a dependency to this function
					httpAuth := httpauth.NewServer(httpAuthAddress, provider, ctxprovider.NewContextProvider(dclient))
					// run servers
					group.StartWithContext(ctx, func(ctx context.Context) {
						// probes
						defer cancel()
						for {
							select {
							case <-ctx.Done():
								return
							default:
								if httpErr = http.Run(ctx); httpErr != nil {
									if errors.Is(httpAuthErr, nethttp.ErrServerClosed) {
										logger.Error("error running the probes, sleeping 10 seconds then retrying")
										time.Sleep(time.Second * 10)
										continue
									}
									logger.WithError(err).Error("fatal error running probes server, not retrying")
									return
								}
							}
						}
					})
					group.StartWithContext(ctx, func(ctx context.Context) {
						// auth server
						defer cancel()
						for {
							select {
							case <-ctx.Done():
								return
							default:
								if httpAuthErr = httpAuth.Run(ctx); httpAuthErr != nil {
									if errors.Is(httpAuthErr, nethttp.ErrServerClosed) {
										logger.Error("error running the auth server, sleeping 10 seconds then retrying")
										time.Sleep(time.Second * 10)
										continue
									}
									logger.WithError(err).Error("fatal error running probes server, not retrying")
									return
								}
							}
						}
					})
					group.StartWithContext(ctx, func(ctx context.Context) {
						// control plane connection
						for {
							select {
							case <-ctx.Done():
								return
							default:
								if grpcErr = provider.Start(ctx); grpcErr != nil {
									logger.Error("error connecting to the control plane, sleeping 10 seconds then retrying")
									time.Sleep(time.Second * 10)
								}
								continue
							}
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
	// command.Flags().StringVar(&clientAddr, "client-address", "", "Client address")

	_ = command.MarkFlagRequired("control-plane-address")
	return command
}

func initK8sClients(ctx context.Context, cfg *rest.Config) (dclient.Interface, error) {
	dynamicClient, err := dyn.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	metaClient, err := meta.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	kube, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dclient, err := dclient.NewClient(ctx, dynamicClient, kube, 15*time.Minute, false, metaClient)
	if err != nil {
		return nil, err
	}
	return dclient, nil
}
