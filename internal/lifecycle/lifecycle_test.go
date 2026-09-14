package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// TestStartRunsComponentsInOrderAndStopReversesIt proves the components come
// up in registration order and go down in the opposite one, the way a defer
// stack unwinds: what was started last is stopped first.
func TestStartRunsComponentsInOrderAndStopReversesIt(t *testing.T) {
	resetRegistry(t)

	var events []string
	for _, name := range []string{"first", "second", "third"} {
		Register(recordingComponent(name, &events, nil))
	}

	require.NoError(t, Start(context.Background()))
	require.Equal(t, []string{"start first", "start second", "start third"}, events)

	Stop(context.Background())
	require.Equal(t, []string{"start first", "start second", "start third", "stop third", "stop second", "stop first"}, events)
}

// TestStartStopsAtTheFirstFailure proves a component that fails to start
// halts the sequence: the ones after it never start, the ones before it keep
// running and are the only ones Stop stops.
func TestStartStopsAtTheFirstFailure(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("first", &events, nil))
	Register(recordingComponent("failing", &events, errors.New("sample failure")))
	Register(recordingComponent("third", &events, nil))

	err := Start(context.Background())
	require.ErrorContains(t, err, `failed to start component "failing"`)
	require.ErrorContains(t, err, "sample failure")
	require.Equal(t, []string{"start first", "start failing"}, events)

	Stop(context.Background())
	require.Equal(t, []string{"start first", "start failing", "stop first"}, events)
}

// TestStopRunsOnce proves a second Stop is a no-op, so the shutdown path can
// stop the components early and let the cleanup stack call Stop again.
func TestStopRunsOnce(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("sample", &events, nil))
	require.NoError(t, Start(context.Background()))

	Stop(context.Background())
	Stop(context.Background())
	require.Equal(t, []string{"start sample", "stop sample"}, events)
}

// TestStartRunsOnce proves bootstrap cannot start the components twice: a
// second Start reports an error instead of doubling their work.
func TestStartRunsOnce(t *testing.T) {
	resetRegistry(t)

	var events []string
	Register(recordingComponent("sample", &events, nil))
	require.NoError(t, Start(context.Background()))
	require.ErrorContains(t, Start(context.Background()), "already started")
	require.Equal(t, []string{"start sample"}, events)
}

// TestRegisterRejectsProgrammerErrors proves the registry refuses what it
// could only accept by silently dropping a component: an empty name, a
// missing Start or Stop, a duplicate name, and a registration that comes
// after the components were started.
func TestRegisterRejectsProgrammerErrors(t *testing.T) {
	noop := func(context.Context) error { return nil }
	noopStop := func(context.Context) {}

	cases := []struct {
		name     string
		register func()
		want     string
	}{
		{
			name:     "empty name",
			register: func() { Register(Component{Name: " ", Start: noop, Stop: noopStop}) },
			want:     "non-empty name",
		},
		{
			name:     "missing start",
			register: func() { Register(Component{Name: "sample", Stop: noopStop}) },
			want:     "requires a Start and a Stop",
		},
		{
			name:     "missing stop",
			register: func() { Register(Component{Name: "sample", Start: noop}) },
			want:     "requires a Start and a Stop",
		},
		{
			name: "duplicate name",
			register: func() {
				Register(Component{Name: "sample", Start: noop, Stop: noopStop})
				Register(Component{Name: "sample", Start: noop, Stop: noopStop})
			},
			want: "duplicate component registration",
		},
		{
			name: "after start",
			register: func() {
				require.NoError(t, Start(context.Background()))
				Register(Component{Name: "late", Start: noop, Stop: noopStop})
			},
			want: "registered after bootstrap started the components",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetRegistry(t)
			require.Contains(t, capturePanic(t, tc.register), tc.want)
		})
	}
}

// recordingComponent builds a component that appends its start and stop to
// events, failing to start with startErr when that is non-nil.
func recordingComponent(name string, events *[]string, startErr error) Component {
	return Component{
		Name: name,
		Start: func(context.Context) error {
			*events = append(*events, "start "+name)
			return startErr
		},
		Stop: func(context.Context) {
			*events = append(*events, "stop "+name)
		},
	}
}

// capturePanic runs fn and returns the message it panics with, failing the
// test when it returns normally.
func capturePanic(t *testing.T, fn func()) (msg string) {
	t.Helper()

	defer func() {
		if recovered := recover(); recovered != nil {
			msg = fmt.Sprint(recovered)
		}
	}()
	fn()
	t.Fatal("expected a panic")
	return ""
}

// resetRegistry rewinds the package-level registry so each test starts from
// an empty, unsealed one.
func resetRegistry(t *testing.T) {
	t.Helper()

	mu.Lock()
	defer mu.Unlock()
	sealed = false
	stopped = false
	components = nil
	started = nil
}
