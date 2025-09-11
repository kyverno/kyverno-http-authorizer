package serve

import (
	authzserver "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/authz-server"
	sidecarinjector "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/sidecar-injector"
	validationwebhook "github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve/validation-webhook"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	command := &cobra.Command{
		Use:   "serve",
		Short: "Run Kyverno Envoy Plugin servers",
	}
	command.AddCommand(authzserver.Command())
	command.AddCommand(sidecarinjector.Command())
	command.AddCommand(validationwebhook.Command())
	return command
}
