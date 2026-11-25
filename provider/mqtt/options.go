package mqtt

import "time"

// PublishOption is how Publish sends a message: its quality of service,
// whether the broker retains it, and how long to wait for the broker.
type PublishOption struct {
	QoS     byte
	Retain  bool
	Timeout time.Duration
}

// SubscribeOption is how Subscribe takes a topic: the quality of service it
// asks for and how long to wait for the broker.
type SubscribeOption struct {
	QoS     byte
	Timeout time.Duration
}

// DefaultPublishOption is the PublishOption Publish uses when given none.
var DefaultPublishOption = PublishOption{
	QoS:     0,
	Retain:  false,
	Timeout: 5 * time.Second,
}

// DefaultSubscribeOption is the SubscribeOption Subscribe uses when given
// none.
var DefaultSubscribeOption = SubscribeOption{
	QoS:     0,
	Timeout: 5 * time.Second,
}
