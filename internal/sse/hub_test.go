package sse

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// receiveOne reads one event with a bounded wait so a broken hub fails the
// test instead of hanging it.
func receiveOne(t *testing.T, ch <-chan Event) Event {
	t.Helper()
	select {
	case event, ok := <-ch:
		if !ok {
			t.Fatal("Channel was closed while an event was expected")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for an event")
		return Event{}
	}
}

// requireClosed asserts the channel is closed and drained.
func requireClosed(t *testing.T, ch <-chan Event) {
	t.Helper()
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for the channel to close")
		}
	}
}

func TestHub_PublishReachesTopicSubscribers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	first, cancelFirst := hub.Subscribe("orders")
	defer cancelFirst()
	second, cancelSecond := hub.Subscribe("orders")
	defer cancelSecond()
	other, cancelOther := hub.Subscribe("draws")
	defer cancelOther()

	hub.Publish("orders", Event{Event: "orders", Data: "changed"})

	for _, ch := range []<-chan Event{first, second} {
		event := receiveOne(t, ch)
		if event.Data != "changed" {
			t.Errorf("Expected the published event, got %+v", event)
		}
	}
	select {
	case event := <-other:
		t.Errorf("Subscriber of another topic must not receive the event, got %+v", event)
	default:
	}
}

func TestHub_MultiTopicSubscriptionReceivesUnion(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	ch, cancel := hub.Subscribe("orders", "draws")
	defer cancel()

	hub.Publish("orders", Event{Data: "order"})
	hub.Publish("draws", Event{Data: "draw"})
	hub.Publish("bets", Event{Data: "bet"})

	if got := receiveOne(t, ch).Data; got != "order" {
		t.Errorf("Expected the orders event first, got %v", got)
	}
	if got := receiveOne(t, ch).Data; got != "draw" {
		t.Errorf("Expected the draws event second, got %v", got)
	}
	select {
	case event := <-ch:
		t.Errorf("Expected no event from an unsubscribed topic, got %+v", event)
	default:
	}
}

func TestHub_EmptySubscriptionNeverDelivers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	ch, cancel := hub.Subscribe()
	hub.Publish("orders", Event{Data: "order"})

	select {
	case event, ok := <-ch:
		if ok {
			t.Errorf("Expected no delivery on an empty subscription, got %+v", event)
		} else {
			t.Error("Expected the channel to stay open until canceled")
		}
	default:
	}

	cancel()
	requireClosed(t, ch)
}

func TestHub_SlowSubscriberLosesOldestEvents(t *testing.T) {
	hub := NewHub(WithSubscriberBuffer(2))
	defer hub.Close()

	slow, cancelSlow := hub.Subscribe("orders")
	defer cancelSlow()
	fast, cancelFast := hub.Subscribe("orders")
	defer cancelFast()

	// Nobody reads yet: four events against a buffer of two.
	for i := 1; i <= 4; i++ {
		hub.Publish("orders", Event{Data: i})
	}

	// The slow subscriber lost the oldest two events and keeps the newest two.
	if got := receiveOne(t, slow).Data; got != 3 {
		t.Errorf("Expected the oldest surviving event to be 3, got %v", got)
	}
	if got := receiveOne(t, slow).Data; got != 4 {
		t.Errorf("Expected the newest event to be 4, got %v", got)
	}

	// The fast subscriber has its own buffer and lost the same way; drain it
	// to show one subscriber's slowness never blocks the publisher.
	if got := receiveOne(t, fast).Data; got != 3 {
		t.Errorf("Expected the fast subscriber to hold event 3, got %v", got)
	}

	if stats := hub.Stats(); stats.Dropped != 4 {
		t.Errorf("Expected 4 dropped events (2 per subscriber), got %d", stats.Dropped)
	}
}

func TestHub_CancelStopsDeliveryAndClosesChannel(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	ch, cancel := hub.Subscribe("orders")
	cancel()
	cancel() // idempotent

	requireClosed(t, ch)
	hub.Publish("orders", Event{Data: "after cancel"}) // must not panic

	if stats := hub.Stats(); stats.Subscribers != 0 {
		t.Errorf("Expected no live subscribers after cancel, got %d", stats.Subscribers)
	}
}

func TestHub_CloseShutsEverythingDown(t *testing.T) {
	hub := NewHub()

	first, cancelFirst := hub.Subscribe("orders")
	defer cancelFirst()
	second, _ := hub.Subscribe("orders", "draws")

	hub.Close()
	hub.Close() // idempotent

	requireClosed(t, first)
	requireClosed(t, second)

	hub.Publish("orders", Event{Data: "after close"}) // no-op, must not panic
	cancelFirst()                                     // canceling after Close must not panic

	late, cancelLate := hub.Subscribe("orders")
	requireClosed(t, late)
	cancelLate()

	stats := hub.Stats()
	if stats.Subscribers != 0 {
		t.Errorf("Expected no subscribers on a closed hub, got %d", stats.Subscribers)
	}
	if stats.Published != 0 {
		t.Errorf("Expected publishes on a closed hub to go uncounted, got %d", stats.Published)
	}
}

func TestHub_StatsCountsDistinctSubscribers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	_, cancelMulti := hub.Subscribe("orders", "draws")
	defer cancelMulti()
	_, cancelSingle := hub.Subscribe("orders")
	defer cancelSingle()

	stats := hub.Stats()
	if stats.Subscribers != 2 {
		t.Errorf("Expected 2 distinct subscribers, got %d", stats.Subscribers)
	}

	hub.Publish("orders", Event{Data: "one"})
	hub.Publish("missing", Event{Data: "two"})

	stats = hub.Stats()
	if stats.Published != 2 {
		t.Errorf("Expected 2 publish calls counted, got %d", stats.Published)
	}
}

func TestHub_RejectsInvalidBuffer(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Expected NewHub to panic on a non-positive buffer capacity")
		}
	}()
	NewHub(WithSubscriberBuffer(0))
}

func TestHub_ConcurrentPublishSubscribeCancelClose(t *testing.T) {
	hub := NewHub(WithSubscriberBuffer(4))

	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			topic := fmt.Sprintf("topic-%d", worker%3)
			for i := range 200 {
				ch, cancel := hub.Subscribe(topic)
				hub.Publish(topic, Event{Data: i})
				select {
				case <-ch:
				default:
				}
				cancel()
			}
		}(worker)
	}
	wg.Wait()
	hub.Close()

	if stats := hub.Stats(); stats.Subscribers != 0 {
		t.Errorf("Expected no subscribers left, got %d", stats.Subscribers)
	}
}
