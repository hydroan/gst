package config

const (
	POSTGRES_HOST     = "POSTGRES_HOST"     //nolint:staticcheck
	POSTGRES_PORT     = "POSTGRES_PORT"     //nolint:staticcheck
	POSTGRES_DATABASE = "POSTGRES_DATABASE" //nolint:staticcheck
	POSTGRES_USERNAME = "POSTGRES_USERNAME" //nolint:staticcheck
	POSTGRES_PASSWORD = "POSTGRES_PASSWORD" //nolint:staticcheck,gosec
	POSTGRES_SSLMODE  = "POSTGRES_SSLMODE"  //nolint:staticcheck
	POSTGRES_TIMEZONE = "POSTGRES_TIMEZONE" //nolint:staticcheck
	POSTGRES_ENABLE   = "POSTGRES_ENABLE"   //nolint:staticcheck
)

type Postgres struct {
	Host     string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port     uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	SSLMode  string `json:"sslmode" mapstructure:"sslmode" ini:"sslmode" yaml:"sslmode"`
	TimeZone string `json:"timezone" mapstructure:"timezone" ini:"timezone" yaml:"timezone"`
	Enable   bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Postgres) setDefault() {
	cv.SetDefault("postgres.host", "127.0.0.1")
	cv.SetDefault("postgres.port", 5432)
	cv.SetDefault("postgres.database", "postgres")
	cv.SetDefault("postgres.username", "postgres")
	cv.SetDefault("postgres.password", "")
	cv.SetDefault("postgres.sslmode", "disable")
	cv.SetDefault("postgres.timezone", "UTC")
	cv.SetDefault("postgres.enable", true)
}
