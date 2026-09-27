// Package configx registers the application's own configuration sections:
// a struct per section, in a file of its own, read like the framework's own
// sections — the environment first, the config file next, the default tags
// last.
package configx

import "github.com/hydroan/gst/config"

func init() {
	config.Register[Notice]()
	config.Register[Cleanup]()
}
