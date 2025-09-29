package authzserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hairyhenderson/go-fsimpl"
	"github.com/hairyhenderson/go-fsimpl/filefs"
	"github.com/hairyhenderson/go-fsimpl/gitfs"
	"github.com/kyverno/kyverno-http-authorizer/pkg/cel/ctxprovider"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine/providers"
	genericproviders "github.com/kyverno/kyverno-http-authorizer/pkg/engine/providers"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/httpauth"
	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/listener"
	"k8s.io/client-go/dynamic"

	nethttp "net/http"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
)

func Command() *cobra.Command {
	var probesAddress string
	var httpAuthAddress string
	var controlPlaneAddr string
	var controlPlaneReconnectWait time.Duration
	var controlPlaneMaxDialInterval time.Duration
	var healthCheckInterval time.Duration
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

					// initialize kubernetes client
					dyn, err := dynamic.NewForConfig(cfg)
					if err != nil {
						return err
					}

					vpolCompiler := vpolcompiler.NewCompiler()

					allProviders, err := getExternalProviders(vpolCompiler)
					if err != nil {
						return err
					}

					provider := listener.NewPolicyListener(controlPlaneAddr,
						clientAddr, vpolCompiler,
						logger, controlPlaneReconnectWait,
						controlPlaneMaxDialInterval,
						healthCheckInterval)

					allProviders = append(allProviders, provider)
					composite := providers.NewComposite(allProviders...)

					// create http and grpc server
					http := probes.NewServer(probesAddress)
					a := httpauth.NewAuthorizer(ctxprovider.NewContextProvider(dyn), composite, logger)
					httpAuth := httpauth.NewServer(httpAuthAddress, provider, a)
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
	command.Flags().DurationVar(&controlPlaneReconnectWait, "control-plane-reconnect-wait", 3*time.Second, "Duration to wait before retrying connecting to the control plane")
	command.Flags().DurationVar(&controlPlaneMaxDialInterval, "control-plane-max-dial-interval", 8*time.Second, "Duration to wait before stopping attempts of sending a policy to a client")
	command.Flags().DurationVar(&healthCheckInterval, "health-check-interval", 30*time.Second, "Interval for sending health checks")
	command.Flags().StringVar(&probesAddress, "probes-address", ":9088", "Address to listen on for health checks")
	command.Flags().StringVar(&httpAuthAddress, "http-auth-server-address", ":9083", "Address to serve the http authorization server on")
	command.Flags().StringVar(&controlPlaneAddr, "control-plane-address", "", "Control plane address")
	// command.Flags().StringVar(&clientAddr, "client-address", "", "Client address")

	_ = command.MarkFlagRequired("control-plane-address")
	return command
}

func getExternalProviders(vpolCompiler vpolcompiler.Compiler, urls ...string) ([]engine.Provider, error) {
	mux := fsimpl.NewMux()
	mux.Add(filefs.FS)
	mux.Add(gitfs.FS)
	var providers []engine.Provider
	for _, url := range urls {
		fsys, err := mux.Lookup(url)
		if err != nil {
			return nil, err
		}
		providers = append(providers, genericproviders.NewFsProvider(vpolCompiler, fsys))
	}
	return providers, nil
}
