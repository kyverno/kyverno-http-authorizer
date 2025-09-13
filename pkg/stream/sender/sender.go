package sender

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/kyverno/kyverno-http-authorizer/apis/v1alpha1"
	"github.com/kyverno/kyverno-http-authorizer/pkg/stream"
	protov1alpha1 "github.com/kyverno/kyverno-http-authorizer/proto/validatingpolicy/v1alpha1"
	"github.com/sirupsen/logrus"
	"go.uber.org/multierr"
	"google.golang.org/grpc"
)

type PolicySender struct {
	protov1alpha1.UnimplementedValidatingPolicyServiceServer
	polMu    *sync.Mutex
	policies map[string]*v1alpha1.ValidatingPolicy

	cxnMu   *sync.Mutex
	cxnsMap map[string]grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]

	ctx                   context.Context
	logger                *logrus.Logger
	initialSendPolicyWait int
	maxSendPolicyInterval int
	sortPolicies          func() []*v1alpha1.ValidatingPolicy
}

func NewPolicySender(ctx context.Context, logger *logrus.Logger) *PolicySender {
	return &PolicySender{
		polMu:                 &sync.Mutex{},
		cxnMu:                 &sync.Mutex{},
		logger:                logger,
		ctx:                   ctx,
		policies:              make(map[string]*v1alpha1.ValidatingPolicy),
		cxnsMap:               make(map[string]grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy]),
		initialSendPolicyWait: 5,
		maxSendPolicyInterval: 10,
	}
}

func (s *PolicySender) SendPolicy(pol *v1alpha1.ValidatingPolicy) error {
	erroredClients := []error{}
	for _, stream := range s.cxnsMap {
		if err := s.sendWithBackoff(stream, pol); err != nil {
			erroredClients = append(erroredClients, err)
		}
	}
	return multierr.Combine(erroredClients...)
}

func (s *PolicySender) StorePolicy(pol *v1alpha1.ValidatingPolicy) {
	// this function just sets the struct field, it gets executed when the policies are being fetched
	// so there is no double locking
	resetSortPolicies := func() {
		s.sortPolicies = sync.OnceValue(func() []*v1alpha1.ValidatingPolicy {
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
			if err != nil { // ammar: theres a panic here. do something about it
				s.logger.Infof("Error receiving response from %s: %v", req.ClientAddress, err)
				return nil
			}

			s.logger.Infof("Received validating policy stream request from: %s", req.ClientAddress)
			if _, ok := s.cxnsMap[req.ClientAddress]; !ok {
				s.cxnMu.Lock()
				s.cxnsMap[req.ClientAddress] = stream
				s.cxnMu.Unlock()
				for _, pol := range s.sortPolicies() {
					if err := s.sendWithBackoff(stream, pol); err != nil {
						s.logger.Errorf("failed to send policy %s to client %s: %s", pol.Name, req.ClientAddress, err)
					}
				}
			}
			// ammar: what to do in case this sender exists?
		}
	}
}

func (s *PolicySender) sendWithBackoff(stream grpc.BidiStreamingServer[protov1alpha1.ValidatingPolicyStreamRequest, protov1alpha1.ValidatingPolicy], pol *v1alpha1.ValidatingPolicy) error {
	operation := func() error {
		if err := stream.Send(v1alpha1.ToProto(pol)); err != nil {
			return err
		}
		return nil
	}
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = time.Duration(s.initialSendPolicyWait) * time.Second
	b.MaxInterval = time.Duration(s.maxSendPolicyInterval) * time.Second
	return backoff.Retry(operation, b)
}
