package httpauth

import (
	"context"
	"net/http"

	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"github.com/kyverno/kyverno-http-authorizer/pkg/server"
	"github.com/kyverno/kyverno/pkg/cel/libs/resource"
	"github.com/sirupsen/logrus"
)

func NewServer(addr string, provider engine.Provider, resourceCtx resource.ContextInterface) server.ServerFunc {
	return func(ctx context.Context) error {
		// create mux
		mux := http.NewServeMux()
		// register health check
		a := &Authorizer{
			provider:    provider,
			logger:      logrus.New(),
			resourceCtx: resourceCtx,
		}

		mux.Handle("POST /", http.HandlerFunc(a.NewHandler()))
		// create server
		s := &http.Server{
			Addr:    addr,
			Handler: mux,
		}
		// run server
		return server.RunHttp(ctx, s, "", "")
	}
}
