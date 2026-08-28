package mqtt_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/hydroan/gst/bootstrap"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/provider/mqtt"
	"github.com/hydroan/gst/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMqtt(t *testing.T) {
	t.Skip("MQTT provider integration is temporarily disabled: it needs a live broker.")

	config.SetConfigFile("../../examples/demo/config.ini")
	util.RunOrDie(bootstrap.Bootstrap)
	defer func() { _ = mqtt.Close() }()

	require.NoError(t, mqtt.Health())

	topic := "test/topic"
	t.Run("PublishAndSubscribe", func(t *testing.T) {
		message := map[string]any{
			"name": "test",
			"time": time.Now().Unix(),
		}
		var wg sync.WaitGroup
		wg.Add(1)

		var received []byte
		var receivedTopic string

		// subscript
		require.NoError(t, mqtt.Subscribe(topic, func(topic string, payload []byte) error {
			received = payload
			receivedTopic = topic
			wg.Done()
			return nil
		}))

		// public
		require.NoError(t, mqtt.Publish(topic, message))
		done := make(chan struct{})

		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			assert.Equal(t, topic, receivedTopic)
			var receivedMsg map[string]any
			err := json.Unmarshal(received, &receivedMsg)
			require.NoError(t, err)
			assert.Equal(t, message["name"], receivedMsg["name"])
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for message")
		}
	})

	t.Run("PublishWithOptions", func(t *testing.T) {
		topic := "test/qos1"
		message := "test message with qos 1"

		// publish with QoS 1
		err := mqtt.Publish(topic, message, mqtt.PublishOption{
			QoS:     1,
			Retain:  true,
			Timeout: 5 * time.Second,
		})
		require.NoError(t, err)
	})

	t.Run("MultipleSubscriptions", func(t *testing.T) {
		topics := []string{
			"test/multiple/1",
			"test/multiple/2",
		}

		var wg sync.WaitGroup
		wg.Add(len(topics))

		receivedCount := 0
		var mu sync.Mutex

		// subscribe to multiple topics
		for _, topic := range topics {
			err := mqtt.Subscribe(topic, func(topic string, payload []byte) error {
				mu.Lock()
				receivedCount++
				mu.Unlock()
				wg.Done()
				return nil
			})
			require.NoError(t, err)
		}

		// publish a message to every topic
		for _, topic := range topics {
			err := mqtt.Publish(topic, "test message")
			require.NoError(t, err)
		}

		// wait until every message has been received
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			mu.Lock()
			assert.Equal(t, len(topics), receivedCount)
			mu.Unlock()
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for messages")
		}
	})

	t.Run("UnsubscribeTest", func(t *testing.T) {
		topic := "test/unsubscribe"

		// subscribe first
		err := mqtt.Subscribe(topic, func(topic string, payload []byte) error {
			t.Error("should not receive message after unsubscribe")
			return nil
		})
		require.NoError(t, err)

		// then unsubscribe
		err = mqtt.Unsubscribe(topic)
		require.NoError(t, err)

		// publish a message
		err = mqtt.Publish(topic, "test message")
		require.NoError(t, err)

		// wait a while to make sure no message arrives
		// time.Sleep(2 * time.Second)
	})

	// t.Run("ErrorCases", func(t *testing.T) {
	// 	// an invalid topic
	// 	err := mqtt.Publish("", "test message")
	// 	assert.Error(t, err)
	//
	// 	// an invalid QoS
	// 	err = mqtt.Publish("test/topic", "test message", mqtt.PublishOption{
	// 		QoS: 3, // invalid QoS value
	// 	})
	// 	assert.Error(t, err)
	//
	// 	// a timeout
	// 	err = mqtt.Publish("test/topic", "test message", mqtt.PublishOption{
	// 		Timeout: 1 * time.Nanosecond,
	// 	})
	// 	assert.Error(t, err)
	// })

	t.Run("JSONPayload", func(t *testing.T) {
		topic := "test/json"
		payload := struct {
			Name string    `json:"name"`
			Age  int       `json:"age"`
			Time time.Time `json:"time"`
		}{
			Name: "test user",
			Age:  25,
			Time: time.Now(),
		}

		var wg sync.WaitGroup
		wg.Add(1)

		err := mqtt.Subscribe(topic, func(topic string, data []byte) error {
			var received struct {
				Name string    `json:"name"`
				Age  int       `json:"age"`
				Time time.Time `json:"time"`
			}
			err := json.Unmarshal(data, &received)
			require.NoError(t, err)
			assert.Equal(t, payload.Name, received.Name)
			assert.Equal(t, payload.Age, received.Age)
			wg.Done()
			return nil
		})
		require.NoError(t, err)

		err = mqtt.Publish(topic, payload)
		require.NoError(t, err)

		// wait for the message
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// success
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for json message")
		}
	})
}
