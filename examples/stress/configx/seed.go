package configx

import "github.com/hydroan/gst/config"

// SEED_ROOT_PASSWORD is the environment variable of Seed.RootPassword.
const SEED_ROOT_PASSWORD = "SEED_ROOT_PASSWORD"

// Seed is the [seed] section: the password the root account is seeded with
// at startup. The process refuses to start without one.
type Seed struct {
	RootPassword string `json:"root_password" mapstructure:"root_password"`
}

func init() {
	config.Register[Seed]()
}
