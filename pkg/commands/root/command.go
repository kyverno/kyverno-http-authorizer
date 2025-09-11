package root

import (
	"github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	root := &cobra.Command{
		Use:   "kyverno-envoy-plugin",
		Short: "kyverno-envoy-plugin is a plugin for Envoy",
	}
	root.AddCommand(serve.Command())
	return root
}
