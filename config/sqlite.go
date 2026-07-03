package config

const (
	SQLITE_PATH      = "SQLITE_PATH"      //nolint:staticcheck
	SQLITE_DATABASE  = "SQLITE_DATABASE"  //nolint:staticcheck
	SQLITE_IS_MEMORY = "SQLITE_IS_MEMORY" //nolint:staticcheck
	SQLITE_ENABLED   = "SQLITE_ENABLED"   //nolint:staticcheck
)

type Sqlite struct {
	Path     string `json:"path" mapstructure:"path" ini:"path" yaml:"path"`
	Database string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	IsMemory bool   `json:"is_memory" mapstructure:"is_memory" ini:"is_memory" yaml:"is_memory"`
	Enabled  bool   `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*Sqlite) setDefault() {
	cv.SetDefault("sqlite.path", "./data.db")
	cv.SetDefault("sqlite.database", "main")
	cv.SetDefault("sqlite.is_memory", true)
	cv.SetDefault("sqlite.enabled", true)
}
