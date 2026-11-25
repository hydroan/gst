package mqtt

import "time"

// PublishOptions is how Publish sends a message: its quality of service,
// whether the broker retains it, and how long to wait for the broker.
type PublishOptions struct {
	QoS     byte
	Retain  bool
	Timeout time.Duration
}

// SubscribeOptions is how Subscribe takes a topic: the quality of service it
// asks for and how long to wait for the broker.
type SubscribeOptions struct {
	QoS     byte
	Timeout time.Duration
}

// DefaultPublishOptions is the PublishOptions Publish uses when given none.
var DefaultPublishOptions = PublishOptions{
	QoS:     0,
	Retain:  false,
	Timeout: 5 * time.Second,
}

// DefaultSubscribeOptions is the SubscribeOptions Subscribe uses when given
// none.
var DefaultSubscribeOptions = SubscribeOptions{
	QoS:     0,
	Timeout: 5 * time.Second,
}
