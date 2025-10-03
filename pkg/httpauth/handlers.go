package httpauth

import (
	"bufio"
	"context"
	"fmt"
	"net/http"

	httpauth "github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/http"
	httpcel "github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/http"
	"github.com/kyverno/kyverno/pkg/cel/libs/resource"
	"github.com/sirupsen/logrus"

	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
)

type authorizer struct {
	provider      engine.Provider
	logger        *logrus.Logger
	resourceCtx   resource.ContextInterface
	nestedRequest bool
}

func NewAuthorizer(ctx resource.ContextInterface, p engine.Provider, nestedRequest bool, logger *logrus.Logger) *authorizer {
	return &authorizer{
		provider:      p,
		logger:        logger,
		resourceCtx:   ctx,
		nestedRequest: nestedRequest,
	}
}

func (a *authorizer) NewHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		a.logger.Infof("received request from %s", r.RemoteAddr)
		if a.nestedRequest {
			reader := bufio.NewReader(r.Body)
			req, err := http.ReadRequest(reader)
			if err != nil {
				writeErrResp(w, err)
				return
			}
			r = req
		}

		pols, err := a.provider.CompiledPolicies(context.Background())
		if err != nil {
			writeErrResp(w, err)
			return
		}
		ruleFuncs := []engine.RequestFunc{}
		httpReq, err := httpauth.NewRequest(r)
		if err != nil {
			writeErrResp(w, err)
			return
		}
		for _, pol := range pols {
			ruleFuncs = append(ruleFuncs, pol.ForHTTP(a.resourceCtx, &httpReq))
		}
		for _, r := range ruleFuncs {
			resp, err := r()
			if err != nil {
				writeErrResp(w, err)
				return
			}
			// write the first valid policy response and exit
			if resp != nil {
				writeResponse(w, resp)
				return
			}
		}
	}
}

func writeErrResp(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprint(w, err.Error()) //nolint:errcheck
}

func writeResponse(w http.ResponseWriter, resp *httpcel.Response) {
	if resp.Headers != nil {
		for k, v := range resp.Headers.GetInnerMap() {
			for _, val := range v {
				w.Header().Set(k, val)
			}
		}
	}

	w.WriteHeader(resp.Status)
	fmt.Fprint(w, resp.Body) //nolint:errcheck
}
