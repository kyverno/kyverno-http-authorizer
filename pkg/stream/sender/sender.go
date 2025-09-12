package sender

import (
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	"google.golang.org/grpc"
)

type PolicySender struct {
	clients    map[string]grpc.ClientConn
	policyChan chan engine.CompiledPolicy
}
