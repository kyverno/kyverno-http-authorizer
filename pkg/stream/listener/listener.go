package listener

import (
	"cmp"
	"context"
	"io"
	"slices"
	"sync"

	"github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/engine"
	vpolcompiler "github.com/kyverno/kyverno-http-authorizer/pkg/engine/vpol/compiler"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PolicyListener struct {
	controlPlaneAddr string
	client           protov1alpha1.ValidatingPolicyServiceClient
	conn             *grpc.ClientConn
	stream           grpc.BidiStreamingClient[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup // ammar: check if you can remove this wait group
	compiler         vpolcompiler.Compiler
	mu               *sync.Mutex
	policies         map[string]engine.CompiledPolicy
	sortPolicies     func() []engine.CompiledPolicy
	logger           *logrus.Logger
}

func NewPolicyListener(ctx context.Context, cancel context.CancelFunc, controlPlaneAddr string, compiler vpolcompiler.Compiler, logger *logrus.Logger) *PolicyListener {
	return &PolicyListener{
		ctx:              ctx,
		cancel:           cancel,
		controlPlaneAddr: controlPlaneAddr,
		compiler:         compiler,
		logger:           logger,
		mu:               &sync.Mutex{},
	}
}

// ammar: check if the interface needs to remain the same
func (l *PolicyListener) CompiledPolicies(ctx context.Context) ([]engine.CompiledPolicy, error) {
	return l.sortPolicies(), nil
}

func (l *PolicyListener) Start() error {
	err := l.dial()
	if err != nil {
		return err
	}
	if err := l.listen(context.Background()); err != nil {
		return err
	}
	return nil
}

func (l *PolicyListener) Stop() {
	l.logger.Info("Stopping policy receiver...")
	if l.cancel != nil {
		l.cancel()
	}

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
				if err := stream.Send(&protov1alpha1.ValidatingPolicyStreamRequest{ClientAddress: "test:9092"}); err != nil { // ammar: get client address properly
					l.logger.Error("Error sending to stream")
					return
				}
				req, err := l.stream.Recv()
				if err == io.EOF {
					l.logger.Info("Policy sender closed the stream")
					return
				}
				if err != nil {
					l.logger.Infof("Error receiving policy request: %v", err)
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
	resetSortPolicies := func() {
		l.sortPolicies = sync.OnceValue(func() []engine.CompiledPolicy {
			l.mu.Lock()
			defer l.mu.Unlock()
			return mapToSortedSlice(l.policies)
		})
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

func mapToSortedSlice[K cmp.Ordered, V any](in map[K]V) []V {
	if in == nil {
		return nil
	}
	out := make([]V, 0, len(in))
	keys := maps.Keys(in)
	slices.Sort(keys)
	for _, key := range keys {
		out = append(out, in[key])
	}
	return out
}
