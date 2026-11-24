// Package service holds what stands behind the public service package: the
// base every service embeds, with the hooks it defaults and the query helpers
// it lends, the registry of the services the controllers run, and the
// detection of the hooks a service overrides.
//
// Application code should use github.com/hydroan/gst/service instead. Keeping
// lookup and registry state in an internal package prevents external projects
// from mutating framework-owned service mappings.
//
// The package-level API is intentionally small:
//   - Register registers a service by its type, the registration the public
//     service.Register forwards to.
//   - RegisterInstance stores a concrete service instance for module/runtime
//     registration.
//   - Resolve returns the service a controller should execute, under the key
//     Key gives a phase on a route.
//   - OverridesHook reports whether a service declares a hook of its own.
//   - Init fills loggers for services registered before logger setup.
package service
