package configx

import "github.com/hydroan/gst/config"

// SEED_PASSWORD is the environment variable of Seed.Password.
const SEED_PASSWORD = "SEED_PASSWORD"

// Seed is the [seed] section: the password the reader account is seeded with
// at startup. The process refuses to start without one.
type Seed struct {
	Password string `json:"password" mapstructure:"password"`
}

func init() {
	config.Register[Seed]()
}
