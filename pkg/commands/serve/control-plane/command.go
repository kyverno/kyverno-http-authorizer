package controlplane

import (
	"context"
	"fmt"

	"github.com/hairyhenderson/go-fsimpl"
	"github.com/hairyhenderson/go-fsimpl/filefs"
	"github.com/hairyhenderson/go-fsimpl/gitfs"
	"github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/authz"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	genericproviders "github.com/kyverno/kyverno-http-authorizer/pkg/engine/providers"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	vpolprovider "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/provider"

	"github.com/kyverno/kyverno-http-authorizer/pkg/probes"
	"github.com/kyverno/kyverno-http-authorizer/pkg/signals"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream/sender"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

func Command() *cobra.Command {
	var probesAddress string
	var metricsAddress string
	var grpcAddress string
	var grpcNetwork string
	var kubeConfigOverrides clientcmd.ConfigOverrides
	var externalPolicySources []string
	var kubePolicySource bool
	var initialSendPolicyWait int
	var maxSendPolicyInterval int
	command := &cobra.Command{
		Use:   "control-plane",
		Short: "Start the Kyverno HTTP authorizer control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			// setup signals aware context
			return signals.Do(context.Background(), func(ctx context.Context) error {
				// track errors
				var httpErr, grpcErr, mgrErr error
				err := func(ctx context.Context) error {
					// create a rest config
					kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
						clientcmd.NewDefaultClientConfigLoadingRules(),
						&kubeConfigOverrides,
					)
					config, err := kubeConfig.ClientConfig()
					if err != nil {
						return err
					}
					// create a cancellable context
					ctx, cancel := context.WithCancel(ctx)
					// cancel context at the end
					defer cancel()
					// create a wait group
					var group wait.Group
					// wait all tasks in the group are over
					defer group.Wait()

					logger := logrus.New()
					s := sender.NewPolicySender(ctx, logger, initialSendPolicyWait, maxSendPolicyInterval)
					// if kube policy source is enabled
					if kubePolicySource {
						// create a controller manager
						scheme := runtime.NewScheme()
						if err := v1alpha1.Install(scheme); err != nil {
							return err
						}
						mgr, err := ctrl.NewManager(config, ctrl.Options{
							Scheme: scheme,
							Metrics: metricsserver.Options{
								BindAddress: metricsAddress,
							},
						})
						if err != nil {
							return fmt.Errorf("failed to construct manager: %w", err)
						}
						// create policy reconciler
						r := vpolprovider.NewPolicyReconciler(mgr.GetClient(), s)
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
					}
					// create http and grpc servers
					http := probes.NewServer(probesAddress)
					grpc := authz.NewServer(grpcNetwork, grpcAddress, s)

					// run servers
					group.StartWithContext(ctx, func(ctx context.Context) {
						// cancel context at the end
						defer cancel()
						httpErr = http.Run(ctx)
					})
					group.StartWithContext(ctx, func(ctx context.Context) {
						// cancel context at the end
						defer cancel()
						grpcErr = grpc.Run(ctx)
					})
					return nil
				}(ctx)
				return multierr.Combine(err, httpErr, grpcErr, mgrErr)
			})
		},
	}
	command.Flags().IntVar(&initialSendPolicyWait, "initial-send-wait", 5, "Duration in seconds to wait before retrying a send to a client")
	command.Flags().IntVar(&maxSendPolicyInterval, "max-send-interval", 10, "Duration in seconds to wait before stopping attempts of sending a policy to a client")
	command.Flags().StringVar(&probesAddress, "probes-address", ":9080", "Address to listen on for health checks")
	command.Flags().StringVar(&grpcAddress, "grpc-address", ":9081", "Address to listen on")
	command.Flags().StringVar(&grpcNetwork, "grpc-network", "tcp", "Network to listen on")
	command.Flags().StringVar(&metricsAddress, "metrics-address", ":9082", "Address to listen on for metrics")
	command.Flags().StringArrayVar(&externalPolicySources, "external-policy-source", nil, "External policy sources")
	command.Flags().BoolVar(&kubePolicySource, "kube-policy-source", true, "Enable in-cluster kubernetes policy source")
	clientcmd.BindOverrideFlags(&kubeConfigOverrides, command.Flags(), clientcmd.RecommendedConfigOverrideFlags("kube-"))
	return command
}

// ammar: bring this back
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
