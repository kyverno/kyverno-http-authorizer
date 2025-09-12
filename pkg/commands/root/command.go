package root

import (
	"github.com/kyverno/kyverno-http-authorizer/pkg/commands/serve"
	"github.com/spf13/cobra"
)

func Command() *cobra.Command {
	root := &cobra.Command{
		Use:   "kyverno-http-authorizer",
		Short: "kyverno-http-authorizer is a plugin for HTTP authorization",
	}
	root.AddCommand(serve.Command())
	return root
}
