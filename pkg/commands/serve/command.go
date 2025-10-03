package serve

import (
	controlplane "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/control-plane"
	sidecarinjector "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/sidecar-injector"
	sidecarauthz "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/sidecar-server"
	standalone "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/standalone-authz"
	validationwebhook "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/validation-webhook"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run Kyverno HTTP Authorizer servers",
	}
	command.AddCommand(sidecarauthz.Command())
	command.AddCommand(sidecarinjector.Command())
	command.AddCommand(validationwebhook.Command())
	command.AddCommand(controlplane.Command())
	command.AddCommand(standalone.Command())
	return command
}
