package listener

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	policyapi "github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PolicyListener struct {
	controlPlaneAddr            string
	clientAddr                  string
	client                      protov1alpha1.ValidatingPolicyServiceClient
	conn                        *grpc.ClientConn
	compiler                    vpolcompiler.Compiler
	mu                          *sync.Mutex
	policies                    map[string]engine.CompiledPolicy
	sortPolicies                func() []engine.CompiledPolicy
	connEstablished             bool
	controlPlaneReconnectWait   time.Duration
	controlPlaneMaxDialInterval time.Duration
	healthCheckInterval         time.Duration
	logger                      *logrus.Logger
}

func NewPolicyListener(
	controlPlaneAddr string,
	clientAddr string,
	compiler vpolcompiler.Compiler,
	logger *logrus.Logger,
	controlPlaneReconnectWait,
	controlPlaneMaxDialInterval,
	healthCheckInterval time.Duration) *PolicyListener {
	return &PolicyListener{
		controlPlaneAddr:            controlPlaneAddr,
		compiler:                    compiler,
		logger:                      logger,
		clientAddr:                  clientAddr,
		mu:                          &sync.Mutex{},
		controlPlaneReconnectWait:   controlPlaneReconnectWait,
		controlPlaneMaxDialInterval: controlPlaneMaxDialInterval,
		healthCheckInterval:         healthCheckInterval,
		policies:                    make(map[string]engine.CompiledPolicy),
		sortPolicies: func() []engine.CompiledPolicy {
			return nil
		},
	}
}

// ammar: check if the interface needs to remain the same
func (l *PolicyListener) CompiledPolicies(ctx context.Context) ([]engine.CompiledPolicy, error) {
	return l.sortPolicies(), nil
}

func (l *PolicyListener) Start(ctx context.Context) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = l.controlPlaneReconnectWait
	b.MaxInterval = l.controlPlaneReconnectWait
	if err := backoff.Retry(l.dial, b); err != nil {
		return err
	}
	go l.sendHealthChecks(ctx) // start sending health checks
	if err := l.listen(ctx); err != nil {
		return err
	}
	return nil
}

func (l *PolicyListener) dial() error {
	l.logger.Infof("Connecting to control plane at %s", l.controlPlaneAddr)
	l.connEstablished = false // set connection to false to mark a new connection
	conn, err := grpc.NewClient(l.controlPlaneAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}

	l.conn = conn
	l.client = protov1alpha1.NewValidatingPolicyServiceClient(conn)
	return nil
}

func (l *PolicyListener) listen(ctx context.Context) error {
	l.logger.Info("Establishing validation channel...")

	// Establish the stream
	stream, err := l.client.ValidatingPoliciesStream(ctx)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				l.logger.Info("Stopping policy listener due to context cancellation")
				stream.CloseSend()

				if l.conn != nil {
					l.conn.Close()
				}
				return
			default:
				if !l.connEstablished {
					if err := stream.Send(&protov1alpha1.ValidatingPolicyStreamRequest{ClientAddress: l.clientAddr}); err != nil {
						l.logger.Error("Error sending to stream")
						return
					}
					l.connEstablished = true
				}
				req, err := stream.Recv()
				if err == io.EOF {
					l.logger.Errorf("Policy sender closed the stream")
					return
				}
				if err != nil {
					l.logger.Errorf("Error receiving policy request: %v", err)
					return
				}

				l.logger.Infof("Received validating policy request: %s, Delete: %t", req.Name, req.Delete)
				go l.processPolicy(req)
			}
		}
	}()

	l.logger.Info("Policy listener running...")
	wg.Wait()
	return nil
}

func (l *PolicyListener) processPolicy(req *protov1alpha1.ValidatingPolicy) {
	// this function just sets the struct field, it gets executed when the policies are being fetched
	// so there is no double locking
	resetSortPolicies := func() {
		l.sortPolicies = sync.OnceValue(func() []engine.CompiledPolicy {
			l.mu.Lock()
			defer l.mu.Unlock()
			return stream.MapToSortedSlice(l.policies)
		})
	}
	if req.Delete {
		l.logger.Info("deleting policy: ", req.Name)
		l.mu.Lock()
		delete(l.policies, req.Name)
		l.mu.Unlock()
		resetSortPolicies()
		return
	}
	vpol := policyapi.FromProto(req)
	compiledPolicy, err := l.compiler.Compile(vpol)
	if err != nil {
		l.logger.Errorf("failed to compile policy %s: %s", req.Name, err)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.policies[req.Name] = compiledPolicy
	resetSortPolicies()
}

func (l *PolicyListener) sendHealthChecks(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(l.healthCheckInterval):
			l.client.HealthCheck(ctx, &protov1alpha1.HealthCheckRequest{
				ClientAddress: l.clientAddr,
				Time:          timestamppb.Now()})
			continue
		}
	}
}
