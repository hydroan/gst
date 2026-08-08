package provider

import (
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

// Tests share the package-level registry, so every test registers providers
// under names unique to that test and asserts membership and relative order
// instead of exact registry content.

func TestRegisterAndRegistered(t *testing.T) {
	errBeta := errors.New("beta init sentinel")
	errAlphaClose := errors.New("alpha close sentinel")

	Register(Provider{Name: "test_register_beta", Init: func() error { return errBeta }})
	Register(Provider{Name: "test_register_alpha", Init: func() error { return nil }, Close: func() error { return errAlphaClose }})

	alpha := registeredByName(t, "test_register_alpha")
	beta := registeredByName(t, "test_register_beta")

	// Registered sorts by name, so alpha must come before beta even though
	// beta registered first.
	require.Less(t, indexOfName(t, "test_register_alpha"), indexOfName(t, "test_register_beta"))

	// The registered entries must carry the caller's hooks untouched.
	require.ErrorIs(t, beta.Init(), errBeta)
	require.NoError(t, alpha.Init())
	require.Nil(t, beta.Close)
	require.ErrorIs(t, alpha.Close(), errAlphaClose)
}

func TestRegisterTrimsName(t *testing.T) {
	Register(Provider{Name: "  test_register_trimmed  ", Init: func() error { return nil }})

	registeredByName(t, "test_register_trimmed")
}

func TestRegisterEmptyNamePanics(t *testing.T) {
	require.PanicsWithValue(t, "provider: register requires a non-empty name", func() {
		Register(Provider{Name: "   ", Init: func() error { return nil }})
	})
}

func TestRegisterNilInitPanics(t *testing.T) {
	require.PanicsWithValue(t, `provider: register requires a non-nil Init for provider "test_register_nil_init"`, func() {
		Register(Provider{Name: "test_register_nil_init"})
	})
}

func TestRegisterDuplicateNamePanics(t *testing.T) {
	Register(Provider{Name: "test_register_duplicate", Init: func() error { return nil }})

	require.PanicsWithValue(t, `provider: duplicate provider registration for name "test_register_duplicate"`, func() {
		// The duplicate carries surrounding whitespace to prove dedup runs
		// on the trimmed name.
		Register(Provider{Name: " test_register_duplicate ", Init: func() error { return nil }})
	})
}

// registeredByName returns the registered provider with the given name and
// fails the test when it is absent.
func registeredByName(t *testing.T, name string) Provider {
	t.Helper()
	for _, p := range Registered() {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("provider %q not found in registry", name)
	return Provider{}
}

// indexOfName returns the position of the named provider in Registered.
func indexOfName(t *testing.T, name string) int {
	t.Helper()
	for i, p := range Registered() {
		if p.Name == name {
			return i
		}
	}
	t.Fatalf("provider %q not found in registry", name)
	return -1
}
