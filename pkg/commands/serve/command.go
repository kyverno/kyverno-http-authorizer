package serve

import (
	authzserver "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/authz-server"
	controlplane "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/control-plane"
	sidecarinjector "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/sidecar-injector"
	validationwebhook "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/validation-webhook"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run Kyverno HTTP Authorizer servers",
	}
	command.AddCommand(authzserver.Command())
	command.AddCommand(sidecarinjector.Command())
	command.AddCommand(validationwebhook.Command())
	command.AddCommand(controlplane.Command())
	return command
}
