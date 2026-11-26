package config

import "github.com/spf13/viper"

const (
	SQLITE_PATH      = "SQLITE_PATH"
	SQLITE_IS_MEMORY = "SQLITE_IS_MEMORY"
)

// Sqlite configures the SQLite database database.type selects: the file at
// Path, or, when IsMemory is set, a database held in memory that every
// process starts empty and loses when it exits.
type Sqlite struct {
	Path     string `json:"path" mapstructure:"path" ini:"path" yaml:"path"`
	IsMemory bool   `json:"is_memory" mapstructure:"is_memory" ini:"is_memory" yaml:"is_memory"`
}

func (*Sqlite) setDefaults(v *viper.Viper) {
	v.SetDefault("sqlite.path", "./data.db")
	v.SetDefault("sqlite.is_memory", false)
}
