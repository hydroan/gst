package sse

import (
	"sync"
	"sync/atomic"
)

// DefaultSubscriberBuffer is the per-subscriber event buffer capacity.
const DefaultSubscriberBuffer = 64

// HubOption configures NewHub.
type HubOption func(*hubOptions)

type hubOptions struct {
	subscriberBuffer int
}

// WithSubscriberBuffer overrides DefaultSubscriberBuffer. The capacity must
// be positive; NewHub panics otherwise, as a non-positive buffer is a
// construction-time programming error.
func WithSubscriberBuffer(capacity int) HubOption {
	return func(o *hubOptions) {
		o.subscriberBuffer = capacity
	}
}

// Hub is an in-process publish/subscribe broker for SSE events, the fan-out
// half of a streaming endpoint: business code publishes an event to a topic,
// every connection subscribed to that topic receives it.
//
// Delivery is best-effort by design — the pattern this serves is "hint the
// client to refetch", where the source of truth stays behind a regular pull
// API. Publish never blocks: a subscriber whose buffer is full loses its
// oldest buffered event first, so slow consumers degrade to fresher data,
// never stall the publisher, and never affect other subscribers. Dropped
// events are counted in Stats.
//
// A Hub is plain in-process state with no background goroutines; create one
// where the owning component lives and share the handle.
type Hub struct {
	mu     sync.RWMutex
	topics map[string]map[*hubSubscriber]struct{}
	closed bool

	subscriberBuffer int

	published atomic.Uint64
	dropped   atomic.Uint64
}

// hubSubscriber is one Subscribe registration: a buffered delivery channel
// and the topics it is filed under. closeOnce guards the channel against the
// cancel/Close race.
type hubSubscriber struct {
	ch        chan Event
	topics    []string
	closeOnce sync.Once
}

// NewHub returns a ready Hub. It panics on invalid options.
func NewHub(opts ...HubOption) *Hub {
	o := hubOptions{subscriberBuffer: DefaultSubscriberBuffer}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.subscriberBuffer <= 0 {
		panic("sse: subscriber buffer capacity must be positive")
	}
	return &Hub{
		topics:           make(map[string]map[*hubSubscriber]struct{}),
		subscriberBuffer: o.subscriberBuffer,
	}
}

// Subscribe registers interest in the given topics and returns the delivery
// channel plus a cancel function. The channel is closed by cancel and by
// Close; a receive loop ranging over it ends cleanly on both. cancel is
// idempotent and safe to call concurrently.
//
// Subscribing to zero topics is legal and yields a channel that never
// delivers: the connection stays open on heartbeats alone. Subscribing on a
// closed hub returns an already-closed channel.
func (h *Hub) Subscribe(topics ...string) (<-chan Event, func()) {
	sub := &hubSubscriber{
		ch:     make(chan Event, h.subscriberBuffer),
		topics: topics,
	}

	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		sub.closeOnce.Do(func() { close(sub.ch) })
		return sub.ch, func() {}
	}
	for _, topic := range topics {
		set := h.topics[topic]
		if set == nil {
			set = make(map[*hubSubscriber]struct{})
			h.topics[topic] = set
		}
		set[sub] = struct{}{}
	}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		for _, topic := range sub.topics {
			if set := h.topics[topic]; set != nil {
				delete(set, sub)
				if len(set) == 0 {
					delete(h.topics, topic)
				}
			}
		}
		h.mu.Unlock()
		// Closing outside the lock is safe: the subscriber is no longer
		// reachable from any topic, and in-flight publishes finished when the
		// write lock was granted.
		sub.closeOnce.Do(func() { close(sub.ch) })
	}
	return sub.ch, cancel
}

// Publish delivers the event to every subscriber of the topic. It never
// blocks: a full subscriber first loses its oldest buffered event, and if
// the consumer races the refill, the new event is dropped instead. Publishing
// to a topic without subscribers, or on a closed hub, is a no-op.
func (h *Hub) Publish(topic string, event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.closed {
		return
	}
	h.published.Add(1)
	for sub := range h.topics[topic] {
		select {
		case sub.ch <- event:
			continue
		default:
		}
		// The buffer is full: make room by dropping the oldest event.
		select {
		case <-sub.ch:
			h.dropped.Add(1)
		default:
		}
		select {
		case sub.ch <- event:
		default:
			// The consumer drained and refilled concurrently; count the new
			// event as the dropped one.
			h.dropped.Add(1)
		}
	}
}

// Close shuts the hub down: every subscriber channel is closed, and further
// Publish and Subscribe calls act on a closed hub as documented. Close is
// idempotent.
func (h *Hub) Close() {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	subscribers := make(map[*hubSubscriber]struct{})
	for _, set := range h.topics {
		for sub := range set {
			subscribers[sub] = struct{}{}
		}
	}
	h.topics = make(map[string]map[*hubSubscriber]struct{})
	h.mu.Unlock()

	for sub := range subscribers {
		sub.closeOnce.Do(func() { close(sub.ch) })
	}
}

// HubStats is a point-in-time snapshot of hub activity.
type HubStats struct {
	// Subscribers is the number of distinct live subscriptions.
	Subscribers int
	// Published counts Publish calls since the hub was created.
	Published uint64
	// Dropped counts events lost to full subscriber buffers.
	Dropped uint64
}

// Stats returns a snapshot of hub activity, the raw material for exporting
// metrics wherever the owning component reports its own.
func (h *Hub) Stats() HubStats {
	h.mu.RLock()
	defer h.mu.RUnlock()
	subscribers := make(map[*hubSubscriber]struct{})
	for _, set := range h.topics {
		for sub := range set {
			subscribers[sub] = struct{}{}
		}
	}
	return HubStats{
		Subscribers: len(subscribers),
		Published:   h.published.Load(),
		Dropped:     h.dropped.Load(),
	}
}
