package nats

import (
	"sync"

	natsGo "github.com/nats-io/nats.go"
	"github.com/samsaralc/kratos-transport/broker"
)

type subscriber struct {
	sync.RWMutex

	n       *natsBroker
	s       *natsGo.Subscription
	options broker.SubscribeOptions
	closed  bool
}

func (s *subscriber) Options() broker.SubscribeOptions {
	s.RLock()
	defer s.RUnlock()

	return s.options
}

func (s *subscriber) Topic() string {
	s.RLock()
	defer s.RUnlock()

	if s.s == nil {
		return ""
	}

	return s.s.Subject
}

func (s *subscriber) Unsubscribe(removeFromManager bool) error {
	s.Lock()
	defer s.Unlock()

	s.closed = true

	var err error
	if s.s != nil {
		err = s.s.Unsubscribe()

		if s.n != nil && s.n.subscribers != nil && removeFromManager {
			_ = s.n.subscribers.RemoveOnly(s.s.Subject)
		}
	}

	return err
}

func (s *subscriber) IsClosed() bool {
	s.RLock()
	defer s.RUnlock()

	return s.closed
}
