package config

import "github.com/spf13/viper"

const (
	CLICKHOUSE_HOST         = "CLICKHOUSE_HOST"         //nolint:staticcheck
	CLICKHOUSE_PORT         = "CLICKHOUSE_PORT"         //nolint:staticcheck
	CLICKHOUSE_DATABASE     = "CLICKHOUSE_DATABASE"     //nolint:staticcheck
	CLICKHOUSE_USERNAME     = "CLICKHOUSE_USERNAME"     //nolint:staticcheck
	CLICKHOUSE_PASSWORD     = "CLICKHOUSE_PASSWORD"     //nolint:staticcheck
	CLICKHOUSE_DIAL_TIMEOUT = "CLICKHOUSE_DIAL_TIMEOUT" //nolint:staticcheck
	CLICKHOUSE_READ_TIMEOUT = "CLICKHOUSE_READ_TIMEOUT" //nolint:staticcheck
	CLICKHOUSE_COMPRESS     = "CLICKHOUSE_COMPRESS"     //nolint:staticcheck
	CLICKHOUSE_DEBUG        = "CLICKHOUSE_DEBUG"        //nolint:staticcheck
	CLICKHOUSE_ENABLED      = "CLICKHOUSE_ENABLED"      //nolint:staticcheck
)

// Clickhouse carries the connection options of the analytical instance.
// There is no write timeout: clickhouse-go v2 does not define one (writes are
// bounded by the request context), and it forwards unknown DSN options to the
// server as settings, which rejects the connection over them.
type Clickhouse struct {
	Host        string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port        uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database    string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username    string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password    string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	DialTimeout string `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout string `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	Compress    bool   `json:"compress" mapstructure:"compress" ini:"compress" yaml:"compress"`
	Debug       bool   `json:"debug" mapstructure:"debug" ini:"debug" yaml:"debug"`
	Enabled     bool   `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*Clickhouse) setDefault(v *viper.Viper) {
	v.SetDefault("clickhouse.host", "127.0.0.1")
	v.SetDefault("clickhouse.port", 9000)
	v.SetDefault("clickhouse.database", "default")
	v.SetDefault("clickhouse.username", "default")
	v.SetDefault("clickhouse.password", "")
	v.SetDefault("clickhouse.dial_timeout", "5s")
	v.SetDefault("clickhouse.read_timeout", "30s")
	v.SetDefault("clickhouse.compress", false)
	v.SetDefault("clickhouse.debug", false)
	v.SetDefault("clickhouse.enabled", false)
}
