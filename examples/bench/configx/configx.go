// Package configx registers the application's custom configuration sections.
//
// Declare a struct and call config.Register[T]() in init below. The section
// name is the snake_case of the struct name (Sample -> [sample] in
// config.ini). Fields resolve from environment variables (SAMPLE_ENDPOINT),
// then the config file, then "default" struct tags; see config.Register for
// details.
//
// Example:
//
//	import "github.com/hydroan/gst/config"
//
//	type Sample struct {
//		Endpoint string `json:"endpoint" mapstructure:"endpoint" default:"127.0.0.1:8080"`
//		Enabled  bool   `json:"enabled" mapstructure:"enabled"`
//	}
//
//	func init() {
//		config.Register[Sample]()
//	}
//
//	// Anywhere after startup:
//	cfg := config.Get[Sample]()
package configx

func init() {
	// TODO: register your custom configurations here.
}
