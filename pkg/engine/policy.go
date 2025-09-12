package engine

import (
	"net/http"

	httpcel "github.com/kyverno/kyverno-http-authorizer/pkg/cel/libs/http"
)

type RequestFunc func() (*httpcel.Response, error)

type CompiledPolicy interface {
	ForHTTP(r *http.Request) RequestFunc
}
