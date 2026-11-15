package streamctx_test

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/streamctx"
)

func TestNew_WatchesTheServerOnce(t *testing.T) {
	plain := context.Background()
	if got, stop := streamctx.New(plain); got != plain {
		t.Errorf("Expected a context without a server signal to come back as is")
	} else {
		stop()
	}

	shutdown, beginShutdown := context.WithCancel(context.Background())
	request := streamctx.WithServerShutdown(context.Background(), shutdown)
	stream, stop := streamctx.New(request)
	defer stop()
	if again, stopAgain := streamctx.New(stream); again != stream {
		t.Errorf("Expected a stream context to come back as is when derived again")
	} else {
		stopAgain()
	}

	beginShutdown()
	select {
	case <-stream.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("The stream context must end when the server shuts down")
	}
	if cause := context.Cause(stream); !errors.Is(cause, streamctx.ErrServerShutdown) {
		t.Errorf("Expected ErrServerShutdown as the cause, got %v", cause)
	}
	if request.Err() != nil {
		t.Errorf("Expected the request context itself to run on while the server drains, got %v", request.Err())
	}
}
