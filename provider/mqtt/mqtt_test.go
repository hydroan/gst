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
	config.SetConfigFile("../examples/myproject/config.ini")
	util.RunOrDie(bootstrap.Bootstrap)
	defer mqtt.Close()

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

		// 测试 QoS 1 发布
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

		// 订阅多个主题
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

		// 发布消息到所有主题
		for _, topic := range topics {
			err := mqtt.Publish(topic, "test message")
			require.NoError(t, err)
		}

		// 等待所有消息接收
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

		// 先订阅
		err := mqtt.Subscribe(topic, func(topic string, payload []byte) error {
			t.Error("should not receive message after unsubscribe")
			return nil
		})
		require.NoError(t, err)

		// 取消订阅
		err = mqtt.Unsubscribe(topic)
		require.NoError(t, err)

		// 发布消息
		err = mqtt.Publish(topic, "test message")
		require.NoError(t, err)

		// 等待一段时间，确保没有收到消息
		// time.Sleep(2 * time.Second)
	})

	// t.Run("ErrorCases", func(t *testing.T) {
	// 	// 测试无效的 topic
	// 	err := mqtt.Publish("", "test message")
	// 	assert.Error(t, err)
	//
	// 	// 测试无效的 QoS
	// 	err = mqtt.Publish("test/topic", "test message", mqtt.PublishOption{
	// 		QoS: 3, // 无效的 QoS 值
	// 	})
	// 	assert.Error(t, err)
	//
	// 	// 测试超时情况
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

		// 等待消息接收
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// 成功
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for json message")
		}
	})
}
