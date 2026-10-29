package configx

// SEED_PASSWORD is the environment variable the key of the seed section
// resolves from.
const SEED_PASSWORD = "SEED_PASSWORD"

// Seed configures the accounts the startup seeds, the section [seed] of
// config.ini.
type Seed struct {
	// Password is the password every seeded account starts with.
	Password string `json:"password" mapstructure:"password" default:"12345678"`
}
