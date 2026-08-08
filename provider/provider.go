// Package provider maintains the registry of optional lifecycle providers.
//
// Optional capabilities live in subpackages of this directory. Each
// subpackage registers itself here from its package init function, so a
// project enables a provider simply by importing the package it actually
// uses: the import compiles the capability in, and bootstrap picks it up
// from this registry to drive Init and Close at the right lifecycle points.
// Packages that are never imported are never compiled into the binary.
//
// Business projects may register their own providers the same way to join
// the unified bootstrap lifecycle.
package provider

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

// Provider describes an optional capability that joins the bootstrap
// lifecycle.
type Provider struct {
	// Name uniquely identifies the provider in the registry. Framework
	// providers use their package name (the final import path element) so
	// bootstrap can match compiled-in providers against its known set.
	Name string

	// Init brings the provider up. Bootstrap runs it after core
	// facilities (config, logging, metrics, databases) are ready. Init
	// must be a no-op when the provider is disabled by configuration.
	Init func() error

	// Close releases the provider's resources, mirroring io.Closer.
	// Optional; bootstrap runs it on shutdown when set, logs the returned
	// error, and always continues with the remaining shutdown work.
	Close func() error
}

var (
	mu        sync.Mutex
	sealed    bool
	providers = make(map[string]Provider)
)

// Register adds p to the registry. Registration normally happens in the
// provider package's init function so that importing the package is the
// single act that enables the capability.
//
// An empty name, a nil Init, or a duplicate name panics: all three are
// programmer errors, and skipping or overwriting silently would drop a
// capability the caller compiled in on purpose.
func Register(p Provider) {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		panic("provider: register requires a non-empty name")
	}
	if p.Init == nil {
		panic(fmt.Sprintf("provider: register requires a non-nil Init for provider %q", p.Name))
	}

	mu.Lock()
	defer mu.Unlock()

	if sealed {
		panic(fmt.Sprintf("provider: provider %q registered after bootstrap sealed the registry; register providers in package init functions", p.Name))
	}
	if _, ok := providers[p.Name]; ok {
		panic(fmt.Sprintf("provider: duplicate provider registration for name %q", p.Name))
	}
	providers[p.Name] = p
}

// Seal marks the registry complete. Bootstrap calls it once right after
// draining the registry, so a Register that runs later panics instead of
// being silently ignored: a provider registered after the drain would never
// be initialized. Seal is idempotent. Business code never calls it.
func Seal() {
	mu.Lock()
	defer mu.Unlock()

	sealed = true
}

// Registered returns the registered providers sorted by name.
//
// The order is deterministic so bootstrap initialization is reproducible
// across builds regardless of package init order. Providers must not depend
// on each other, which is what makes name order safe.
func Registered() []Provider {
	mu.Lock()
	defer mu.Unlock()

	list := make([]Provider, 0, len(providers))
	for _, p := range providers {
		list = append(list, p)
	}
	slices.SortFunc(list, func(a, b Provider) int { return strings.Compare(a.Name, b.Name) })
	return list
}
