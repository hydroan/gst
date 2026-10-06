// Package serviceregistry owns the framework-internal service registry used by
// controllers and modules.
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
//   - Resolve returns the service a controller should execute.
//   - Init fills loggers for services registered before logger setup.
package serviceregistry
