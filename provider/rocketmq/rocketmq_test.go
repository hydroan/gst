package rocketmq_test

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/testutil"
	"github.com/hydroan/gst/provider/rocketmq"
	"github.com/stretchr/testify/require"
)

// consumerProcessEnv, when set to "<topic> <group>", makes the test binary
// run as a consumer process of TestPushConsumersInTwoProcessesSplitTheQueues
// instead of running tests.
const consumerProcessEnv = "GST_TEST_ROCKETMQ_CONSUMER_PROCESS"

func TestMain(m *testing.M) {
	if spec := os.Getenv(consumerProcessEnv); spec != "" {
		os.Exit(runConsumerProcess(spec))
	}
	testutil.Run(m, testutil.Server{RocketMQ: true})
}

// TestProducerSends pins that Producer hands out an instance the bootstrap
// started: a message sent through it reaches the broker.
func TestProducerSends(t *testing.T) {
	p, err := rocketmq.Producer()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := p.SendSync(ctx, primitive.NewMessage("sample-topic", []byte("sample")))
	require.NoError(t, err)
	require.Equal(t, primitive.SendOK, result.Status)
}

// TestAdminListsTopics pins that Admin hands out a client that reaches the
// name servers.
func TestAdminListsTopics(t *testing.T) {
	a, err := rocketmq.Admin()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	topics, err := a.FetchAllTopicList(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, topics.TopicList)
}

// TestNewPushConsumerReportsAStartItDoesNotKnow pins that a consume_from_where
// outside its set is an error from NewPushConsumer, before any name server is
// contacted, instead of a consumer started from the last offset. The
// configuration decode does not let such a value through; one set in code
// arrives here.
func TestNewPushConsumerReportsAStartItDoesNotKnow(t *testing.T) {
	_, err := rocketmq.NewPushConsumer(config.RocketMQ{ConsumeFromWhere: "middle"}, "sample-group")
	require.ErrorContains(t, err, "consume_from_where: middle")
}

// TestPushConsumersInTwoProcessesSplitTheQueues pins that two processes on
// one machine, consuming in one group with the framework's default
// configuration, are two consumers to the broker: each message is consumed
// once, by one of them, and both get a share. Each consumer is this test
// binary run again, see runConsumerProcess. Keeping the consumers out of this
// process keeps its race detector off a race inside the client library, whose
// push consumer bumps a subscription's version while its route refresh
// copies the subscription (rocketmq-client-go v2.1.2).
func TestPushConsumersInTwoProcessesSplitTheQueues(t *testing.T) {
	const messages = 40
	suffix := time.Now().UnixNano()
	topic := fmt.Sprintf("sample-split-%d", suffix)
	group := fmt.Sprintf("sample-split-group-%d", suffix)

	// Sending creates the topic with its queues before anyone subscribes.
	p, err := rocketmq.Producer()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = p.SendSync(ctx, primitive.NewMessage(topic, []byte("warmup")))
	require.NoError(t, err)

	var (
		mu       sync.Mutex
		consumed = map[string][]string{}
	)
	record := func(process, body string) {
		mu.Lock()
		defer mu.Unlock()
		consumed[body] = append(consumed[body], process)
	}
	startConsumerProcess(t, "first", topic, group, record)
	startConsumerProcess(t, "second", topic, group, record)
	// Both consumers rebalance once the broker sees the second one; messages
	// sent before that settles could move between them and be consumed twice
	// for that reason alone.
	time.Sleep(10 * time.Second)

	sent := make([]string, 0, messages)
	for i := range messages {
		body := fmt.Sprintf("m-%d", i)
		_, err = p.SendSync(ctx, primitive.NewMessage(topic, []byte(body)))
		require.NoError(t, err)
		sent = append(sent, body)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := 0
		for _, body := range sent {
			if len(consumed[body]) > 0 {
				got++
			}
		}
		mu.Unlock()
		if got == messages {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	// A message consumed twice reaches the second consumer about when it
	// reaches the first.
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	share := map[string]int{}
	for _, body := range sent {
		require.Len(t, consumed[body], 1, "message %s was consumed by %v", body, consumed[body])
		share[consumed[body][0]]++
	}
	require.Positive(t, share["first"], "the first process got no queue")
	require.Positive(t, share["second"], "the second process got no queue")
}

// startConsumerProcess runs this test binary as a consumer process of topic
// in group, see runConsumerProcess, waits until it has started, and hands
// every message it consumes to record under name. The process stops when the
// test ends.
func startConsumerProcess(t *testing.T, name, topic, group string, record func(process, body string)) {
	t.Helper()
	child := exec.Command(os.Args[0], "-test.run=^$")
	child.Env = append(os.Environ(),
		consumerProcessEnv+"="+topic+" "+group,
		config.ROCKETMQ_CONSUME_FROM_WHERE+"="+string(config.ConsumeFromWhereFirstOffset),
	)
	var childErr strings.Builder
	child.Stderr = &childErr
	stdin, err := child.StdinPipe()
	require.NoError(t, err)
	stdout, err := child.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, child.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = child.Wait()
	})

	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case line == "ready":
				close(ready)
			case strings.HasPrefix(line, "consumed "):
				record(name, strings.TrimPrefix(line, "consumed "))
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		t.Fatalf("the %s consumer process did not start: %s", name, childErr.String())
	}
}

// runConsumerProcess is a consumer process: with the framework's default
// configuration, which config.Init reads from no file, it consumes
// the topic named in spec in the group named there, writes "ready" once
// started and "consumed <body>" for every message, and stops when its
// standard input closes.
func runConsumerProcess(spec string) int {
	topic, group, ok := strings.Cut(spec, " ")
	if !ok {
		return 2
	}
	if err := config.Init(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var mu sync.Mutex
	c, err := rocketmq.NewPushConsumer(config.App.RocketMQ, group)
	if err == nil {
		err = c.Subscribe(topic, consumer.MessageSelector{}, func(_ context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
			mu.Lock()
			defer mu.Unlock()
			for _, msg := range msgs {
				fmt.Println("consumed " + string(msg.Body))
			}
			return consumer.ConsumeSuccess, nil
		})
	}
	if err == nil {
		err = c.Start()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("ready")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	_ = c.Shutdown()
	return 0
}
