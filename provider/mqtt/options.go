package mqtt

import "time"

type PublishOption struct {
	QoS     byte
	Retain  bool
	Timeout time.Duration
}

type SubscribeOption struct {
	QoS     byte
	Timeout time.Duration
}

var DefaultPublishOption = PublishOption{
	QoS:     0,
	Retain:  false,
	Timeout: 5 * time.Second,
}

var DefaultSubscribeOption = SubscribeOption{
	QoS:     0,
	Timeout: 5 * time.Second,
}
