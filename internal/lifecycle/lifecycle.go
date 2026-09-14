// Package lifecycle is the registry of the framework components that run
// alongside the HTTP server for the life of the process: the scheduler,
// election loops, anything that owns a background goroutine.
//
// A component registers from its package initialiser, so importing its
// package is the single act that enables it: a project that never imports the
// package never links the component, never starts it and never pays for it.
// Bootstrap starts the registered components once every table they may touch
// exists, right before the HTTP listener opens, and stops them in reverse
// order the moment the process begins to drain — before the listener closes,
// and before any connection they may still be using is torn down.
package lifecycle

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
)

// Component is one framework component with a lifetime of its own.
type Component struct {
	// Name identifies the component in errors and logs.
	Name string
	// Start brings the component up and returns once it is running. It
	// receives the process context, which is canceled when the process
	// begins shutting down, so background work derived from it winds down on
	// its own.
	Start func(ctx context.Context) error
	// Stop halts the component and waits for the work it has in flight, for
	// as long as ctx allows. It runs at most once, and only after Start
	// succeeded.
	Stop func(ctx context.Context)
}

var (
	mu         sync.Mutex
	sealed     bool
	stopped    bool
	components []Component
	started    []Component
)

// Register adds c to the registry. Registration happens in the component
// package's init function, so importing the package is what enables it.
//
// An empty name, a nil Start or Stop, a duplicate name, or a registration
// after bootstrap has started the components panics: each is a programmer
// error, and skipping it silently would drop a component the project compiled
// in on purpose.
func Register(c Component) {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		panic("lifecycle: register requires a non-empty name")
	}
	if c.Start == nil || c.Stop == nil {
		panic(fmt.Sprintf("lifecycle: register requires a Start and a Stop for component %q", c.Name))
	}

	mu.Lock()
	defer mu.Unlock()

	if sealed {
		panic(fmt.Sprintf("lifecycle: component %q registered after bootstrap started the components; register components in package init functions", c.Name))
	}
	if slices.ContainsFunc(components, func(r Component) bool { return r.Name == c.Name }) {
		panic(fmt.Sprintf("lifecycle: duplicate component registration for name %q", c.Name))
	}
	components = append(components, c)
}

// Start starts every registered component in registration order and seals
// the registry. It stops at the first component that fails to start and
// returns that failure; the components started before it keep running until
// Stop. Bootstrap calls it once the tables are ready, and only once: a
// second call is an error, because starting a component twice would double
// its work. Business code never calls it.
func Start(ctx context.Context) error {
	mu.Lock()
	if sealed {
		mu.Unlock()
		return errors.New("lifecycle: components already started")
	}
	sealed = true
	pending := slices.Clone(components)
	mu.Unlock()

	for _, c := range pending {
		if err := c.Start(ctx); err != nil {
			return errors.Wrapf(err, "failed to start component %q", c.Name)
		}
		mu.Lock()
		started = append(started, c)
		mu.Unlock()
	}
	return nil
}

// Stop stops the started components in reverse order, giving each the
// remainder of ctx to finish its in-flight work. A second call is a no-op,
// so bootstrap can stop the components at the first sign of shutdown and let
// its cleanup stack call Stop again without stopping anything twice.
func Stop(ctx context.Context) {
	mu.Lock()
	if stopped {
		mu.Unlock()
		return
	}
	stopped = true
	running := slices.Clone(started)
	mu.Unlock()

	for _, c := range slices.Backward(running) {
		c.Stop(ctx)
	}
}
