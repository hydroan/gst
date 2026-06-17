package config

const (
	CLICKHOUSE_HOST          = "CLICKHOUSE_HOST"          //nolint:staticcheck
	CLICKHOUSE_PORT          = "CLICKHOUSE_PORT"          //nolint:staticcheck
	CLICKHOUSE_DATABASE      = "CLICKHOUSE_DATABASE"      //nolint:staticcheck
	CLICKHOUSE_USERNAME      = "CLICKHOUSE_USERNAME"      //nolint:staticcheck
	CLICKHOUSE_PASSWORD      = "CLICKHOUSE_PASSWORD"      //nolint:staticcheck
	CLICKHOUSE_DIAL_TIMEOUT  = "CLICKHOUSE_DIAL_TIMEOUT"  //nolint:staticcheck
	CLICKHOUSE_READ_TIMEOUT  = "CLICKHOUSE_READ_TIMEOUT"  //nolint:staticcheck
	CLICKHOUSE_WRITE_TIMEOUT = "CLICKHOUSE_WRITE_TIMEOUT" //nolint:staticcheck
	CLICKHOUSE_COMPRESS      = "CLICKHOUSE_COMPRESS"      //nolint:staticcheck
	CLICKHOUSE_DEBUG         = "CLICKHOUSE_DEBUG"         //nolint:staticcheck
	CLICKHOUSE_ENABLE        = "CLICKHOUSE_ENABLE"        //nolint:staticcheck
)

type Clickhouse struct {
	Host         string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port         uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database     string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username     string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password     string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	DialTimeout  string `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	ReadTimeout  string `json:"read_timeout" mapstructure:"read_timeout" ini:"read_timeout" yaml:"read_timeout"`
	WriteTimeout string `json:"write_timeout" mapstructure:"write_timeout" ini:"write_timeout" yaml:"write_timeout"`
	Compress     bool   `json:"compress" mapstructure:"compress" ini:"compress" yaml:"compress"`
	Debug        bool   `json:"debug" mapstructure:"debug" ini:"debug" yaml:"debug"`
	Enable       bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Clickhouse) setDefault() {
	cv.SetDefault("clickhouse.host", "127.0.0.1")
	cv.SetDefault("clickhouse.port", 9000)
	cv.SetDefault("clickhouse.database", "default")
	cv.SetDefault("clickhouse.username", "default")
	cv.SetDefault("clickhouse.password", "")
	cv.SetDefault("clickhouse.dial_timeout", "5s")
	cv.SetDefault("clickhouse.read_timeout", "30s")
	cv.SetDefault("clickhouse.write_timeout", "30s")
	cv.SetDefault("clickhouse.compress", false)
	cv.SetDefault("clickhouse.debug", false)
	cv.SetDefault("clickhouse.enable", false)
}
