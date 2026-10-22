// Package providertest runs the tests of a provider package against the
// external services they need, with the framework brought up as far as its
// provider stage and no further.
package providertest

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/hydroan/gst/internal/lifecycle"
	"github.com/hydroan/gst/internal/testutil/testlog"
	"github.com/hydroan/gst/logger"
)

// Run runs the tests of a provider package. Each prepare function starts a
// service the tests need, publishes its address through the environment the
// way the testcontainer setups do, and returns the function that releases it.
// The framework then takes the steps the bootstrap takes before its provider
// stage — the logs go to a directory of their own, the config and the logger
// load from the environment — and lifecycle.Start brings up the providers the
// binary links and the config enables, the way that stage does. Once the
// tests are over, the providers stop and the services are released, the
// latest first. A service or a provider that fails to come up panics, after
// what came up before it is released. It is the whole body of a provider
// package's TestMain:
//
//	func TestMain(m *testing.M) {
//		providertest.Run(m, testcontainer.SetupMinio)
//	}
//
// A provider's tests run through Run rather than testutil.Run, which brings
// the whole framework up, so that their test binary links the provider and
// what it depends on alone: go test reruns a package whenever its test binary
// changes, and a binary linking the whole framework reruns, starting its
// services again, after a change anywhere in the framework.
//
// Run does not return: it exits the test binary with the result of the tests.
func Run(m *testing.M, prepare ...func() (release func() error, err error)) {
	os.Exit(run(m, prepare))
}

// run holds the body of Run so that the deferred releases still happen: the
// os.Exit in Run would skip them.
func run(m *testing.M, prepare []func() (func() error, error)) int {
	var releases []func() error
	defer func() {
		for _, release := range slices.Backward(releases) {
			if err := release(); err != nil {
				fmt.Fprintf(os.Stderr, "failed to release a test service: %v\n", err)
			}
		}
	}()

	// The logs go to a directory of their own, out of the test output and
	// the package source tree, before anything writes one. Released last, the
	// directory outlives every writer.
	_, releaseLogs, err := testlog.ToTempDir()
	if err != nil {
		panic(err)
	}
	releases = append(releases, releaseLogs)

	// The services come up before config.Init, which reads the addresses
	// they publish through the environment.
	for _, start := range prepare {
		release, err := start()
		if err != nil {
			panic(err)
		}
		releases = append(releases, release)
	}

	if err := config.Init(); err != nil {
		panic(err)
	}
	if err := logger.Init(); err != nil {
		panic(err)
	}
	if err := lifecycle.Start(context.Background(), lifecycle.StageProvider); err != nil {
		panic(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), lifecycle.StopTimeout)
		defer cancel()
		lifecycle.Stop(ctx)
	}()

	return m.Run()
}
