package standalone

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/kyverno/kyverno-http-authorizer/pkg/cel/ctxprovider"
	controlplane "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/control-plane"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine/providers"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	vpolprovider "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/provider"
	"github.com/kyverno/kyverno-http-authorizer/pkg/httpauth"
	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno/api/policies.kyverno.io/v1alpha1"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"

	nethttp "net/http"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func Command() *cobra.Command {
	var probesAddress string
	var httpAuthAddress string
	var kubePolicySource bool
	var kubeConfigOverrides clientcmd.ConfigOverrides
	var externalSources []string
	var metricsAddress string
	command := &cobra.Command{
		Use:   "authz-server",
		Short: "Start the Kyverno Authz Server in standalone mode",
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

					kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
						clientcmd.NewDefaultClientConfigLoadingRules(),
						&kubeConfigOverrides,
					)
					cfg, err := kubeConfig.ClientConfig()
					if err != nil {
						return err
					}

					// initialize kubernetes client
					dyn, err := dynamic.NewForConfig(cfg)
					if err != nil {
						return err
					}

					vpolCompiler := vpolcompiler.NewCompiler()
					extern, err := controlplane.GetExternalProviders(logger, vpolCompiler, externalSources...)
					if err != nil {
						return err
					}
					externalProviders := []engine.Provider{}
					for _, e := range extern {
						externalProviders = append(externalProviders, e)
					}
					provider := providers.NewComposite(externalProviders...)

					if kubePolicySource {
						// create a controller manager
						scheme := runtime.NewScheme()
						if err := v1alpha1.Install(scheme); err != nil {
							return err
						}
						mgr, err := ctrl.NewManager(cfg, ctrl.Options{
							Scheme: scheme,
							Metrics: metricsserver.Options{
								BindAddress: metricsAddress,
							},
						})
						if err != nil {
							return fmt.Errorf("failed to construct manager: %w", err)
						}
						// create policy reconciler
						r := vpolprovider.NewStandaloneReconciler(mgr.GetClient(), vpolCompiler)
						if err := ctrl.NewControllerManagedBy(mgr).For(&v1alpha1.ValidatingPolicy{}).Complete(r); err != nil {
							return fmt.Errorf("failed to register controller to manager: %w", err)
						}
						// start manager
						group.StartWithContext(ctx, func(ctx context.Context) {
							// cancel context at the end
							defer cancel()
							mgrErr = mgr.Start(ctx)
						})
						if !mgr.GetCache().WaitForCacheSync(ctx) {
							defer cancel()
							return fmt.Errorf("failed to wait for cache sync")
						}
						// append the kube provider to the current existing provider
						provider = providers.NewComposite([]engine.Provider{provider, r}...)
					}

					// create http and grpc server
					http := probes.NewServer(probesAddress)
					a := httpauth.NewAuthorizer(ctxprovider.NewContextProvider(dyn), provider, logger)
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
									logger.WithError(err).Error("fatal error running http server, not retrying")
									return
								}
							}
						}
					})
					return nil
				}(ctx)
				return multierr.Combine(err, httpErr, grpcErr, mgrErr, httpAuthErr)
			})
		},
	}
	command.Flags().StringVar(&probesAddress, "probes-address", ":9088", "Address to listen on for health checks")
	command.Flags().StringVar(&httpAuthAddress, "http-auth-server-address", ":9083", "Address to serve the http authorization server on")
	command.Flags().StringVar(&probesAddress, "probes-address", ":9080", "Address to listen on for health checks")
	command.Flags().StringVar(&metricsAddress, "metrics-address", ":9082", "Address to listen on for metrics")
	command.Flags().StringArrayVar(&externalSources, "external-policy-source", nil, "External policy sources")
	clientcmd.BindOverrideFlags(&kubeConfigOverrides, command.Flags(), clientcmd.RecommendedConfigOverrideFlags("kube-"))
	_ = command.MarkFlagRequired("control-plane-address")
	return command
}
