package sender

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"google.golang.org/grpc"
)

type PolicySender struct {
	protov1alpha1.UnimplementedValidatingPolicyServiceServer
	polMu    *sync.Mutex
	policies map[string]*protov1alpha1.ValidatingPolicy

	cxnMu   *sync.Mutex
	cxnsMap map[string]grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]

	ctx                   context.Context
	logger                *logrus.Logger
	initialSendPolicyWait int
	maxSendPolicyInterval int
	sortPolicies          func() []*protov1alpha1.ValidatingPolicy
}

func NewPolicySender(ctx context.Context, logger *logrus.Logger, initialSendPolicyWait, maxSendPolicyInterval int) *PolicySender {
	return &PolicySender{
		polMu:                 &sync.Mutex{},
		cxnMu:                 &sync.Mutex{},
		logger:                logger,
		ctx:                   ctx,
		policies:              make(map[string]*protov1alpha1.ValidatingPolicy),
		cxnsMap:               make(map[string]grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]),
		initialSendPolicyWait: initialSendPolicyWait,
		maxSendPolicyInterval: maxSendPolicyInterval,
	}
}

func (s *PolicySender) SendPolicy(pol *protov1alpha1.ValidatingPolicy) {
	errCh := make(chan error)
	var wg sync.WaitGroup
	wg.Add(len(s.cxnsMap))
	// send to clients, but don't wait on any of them
	for _, stream := range s.cxnsMap {
		go func() {
			defer wg.Done()
			errCh <- s.sendWithBackoff(stream, pol)
		}()
	}

	wg.Wait()
	close(errCh)

	errs := make([]error, len(errCh))
	for e := range errCh {
		errs = append(errs, e)
	}
	if len(errs) > 0 {
		logrus.Error(multierr.Combine(errs...))
	}
}

func (s *PolicySender) StorePolicy(pol *protov1alpha1.ValidatingPolicy) {
	// this function just sets the struct field, it gets executed when the policies are being fetched
	// so there is no double locking
	resetSortPolicies := func() {
		s.sortPolicies = sync.OnceValue(func() []*protov1alpha1.ValidatingPolicy {
			s.polMu.Lock()
			defer s.polMu.Unlock()
			return stream.MapToSortedSlice(s.policies)
		})
	}
	s.polMu.Lock()
	s.policies[pol.Name] = pol
	defer s.polMu.Unlock()
	resetSortPolicies()
}

func (s *PolicySender) DeletePolicy(polName string) {
	resetSortPolicies := func() {
		s.sortPolicies = sync.OnceValue(func() []*protov1alpha1.ValidatingPolicy {
			s.polMu.Lock()
			defer s.polMu.Unlock()
			return stream.MapToSortedSlice(s.policies)
		})
	}
	s.polMu.Lock()
	delete(s.policies, polName)
	defer s.polMu.Unlock()
	resetSortPolicies()
}

func (s *PolicySender) ValidatingPoliciesStream(stream grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]) error {
	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
			req, err := stream.Recv()
			if err == io.EOF {
				s.logger.Infof("Receiver %s closed the stream", req.ClientAddress)
				return nil
			}
			if err != nil {
				s.logger.Infof("Error receiving response: %v", err)
				return err
			}

			s.logger.Infof("Received validating policy stream request from: %s", req.ClientAddress)
			if _, ok := s.cxnsMap[req.ClientAddress]; !ok {
				s.cxnMu.Lock()
				s.cxnsMap[req.ClientAddress] = stream
				s.cxnMu.Unlock()
				if len(s.policies) > 0 {
					for _, pol := range s.sortPolicies() {
						if err := s.sendWithBackoff(stream, pol); err != nil {
							s.logger.Errorf("failed to send policy %s to client %s: %s", pol.Name, req.ClientAddress, err)
						}
					}
				}
			}
			// ammar: what to do in case this sender exists?
		}
	}
}

func (s *PolicySender) sendWithBackoff(stream grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy], pol *protov1alpha1.ValidatingPolicy) error {
	operation := func() error {
		if err := stream.Send(pol); err != nil {
			return err
		}
		return nil
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Duration(s.initialSendPolicyWait) * time.Second
	b.MaxInterval = time.Duration(s.maxSendPolicyInterval) * time.Second
	return backoff.Retry(operation, b)
}
