package listener

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PolicyListener struct {
	controlPlaneAddr            string
	clientAddr                  string
	client                      protov1alpha1.ValidatingPolicyServiceClient
	conn                        *grpc.ClientConn
	stream                      grpc.BidiStreamingClient[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]
	ctx                         context.Context
	wg                          sync.WaitGroup // ammar: check if you can remove this wait group
	compiler                    vpolcompiler.Compiler
	mu                          *sync.Mutex
	policies                    map[string]engine.CompiledPolicy
	sortPolicies                func() []engine.CompiledPolicy
	connEstablished             bool
	controlPlaneReconnectWait   int
	controlPlaneMaxDialInterval int
	logger                      *logrus.Logger
}

func NewPolicyListener(ctx context.Context,
	cancel context.CancelFunc,
	controlPlaneAddr string,
	clientAddr string,
	compiler vpolcompiler.Compiler,
	logger *logrus.Logger,
	controlPlaneReconnectWait int,
	controlPlaneMaxDialInterval int) *PolicyListener {
	return &PolicyListener{
		ctx:                         ctx,
		controlPlaneAddr:            controlPlaneAddr,
		compiler:                    compiler,
		logger:                      logger,
		clientAddr:                  clientAddr,
		mu:                          &sync.Mutex{},
		controlPlaneReconnectWait:   controlPlaneReconnectWait,
		controlPlaneMaxDialInterval: controlPlaneMaxDialInterval,
		policies:                    make(map[string]engine.CompiledPolicy),
	}
}

// ammar: check if the interface needs to remain the same
func (l *PolicyListener) CompiledPolicies(ctx context.Context) ([]engine.CompiledPolicy, error) {
	return l.sortPolicies(), nil
}

func (l *PolicyListener) Start() error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Duration(l.controlPlaneReconnectWait) * time.Second
	b.MaxInterval = time.Duration(l.controlPlaneReconnectWait) * time.Second
	if err := backoff.Retry(l.dial, b); err != nil {
		return err
	}
	if err := l.listen(context.Background()); err != nil {
		return err
	}
	return nil
}

func (l *PolicyListener) Stop() {
	l.logger.Info("Stopping policy receiver...")
	if l.stream != nil {
		l.stream.CloseSend()
	}
	l.wg.Wait()

	if l.conn != nil {
		l.conn.Close()
	}
	l.logger.Info("Policy receiver stopped")
}

func (l *PolicyListener) dial() error {
	l.logger.Infof("Connecting to control plane at %s", l.controlPlaneAddr)
	// Create connection to the control plane
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

	l.stream = stream
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			select {
			case <-l.ctx.Done():
				l.logger.Info("Stopping policy listener due to context cancellation")
				return
			default:
				if !l.connEstablished {
					if err := stream.Send(&protov1alpha1.ValidatingPolicyStreamRequest{ClientAddress: l.clientAddr}); err != nil {
						l.logger.Error("Error sending to stream")
						return
					}
					l.connEstablished = true
				}
				req, err := l.stream.Recv()
				if err == io.EOF {
					l.logger.Errorf("Policy sender closed the stream")
					return
				}
				if err != nil {
					l.logger.Errorf("Error receiving policy request: %v", err)
					return
				}

				l.logger.Infof("Received validating policy request: %s", req.Name)
				go l.processPolicy(req)
			}
		}
	}()

	l.logger.Info("Policy listener running...")
	l.wg.Wait()
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
	// receiving a policy with nil spec means a deletion
	if req.Spec == nil {
		l.logger.Info("deleting policy", req.Name)
		l.mu.Lock()
		delete(l.policies, req.Name)
		l.mu.Unlock()
		resetSortPolicies()
		return
	}
	vpol := v1alpha1.FromProto(req)
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
