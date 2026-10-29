package configx

import "github.com/hydroan/gst/config"

// SEED_ROOT_PASSWORD is the environment variable the one key of the seed
// section resolves from.
const SEED_ROOT_PASSWORD = "SEED_ROOT_PASSWORD"

// Seed holds what the startup seeding reads from configuration, the section
// [seed] of the configuration.
type Seed struct {
	// RootPassword is the password the built-in root account is created with
	// the first time the deployment starts. It has no default: a deployment
	// administers itself through root until it has appointed administrators
	// of its own, so the start refuses to go on without one.
	RootPassword string `json:"root_password" mapstructure:"root_password"`
}

func init() {
	config.Register[Seed]()
}
