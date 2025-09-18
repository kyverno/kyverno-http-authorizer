package engine

import (
	httpcel "github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/http"
	"github.com/kyverno/kyverno/pkg/cel/libs/resource"
)

type RequestFunc func() (*httpcel.Response, error)

type CompiledPolicy interface {
	ForHTTP(ctx resource.ContextInterface, r *httpcel.Request) RequestFunc
}
