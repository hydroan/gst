package config

import "github.com/hydroan/gst/types/consts"

const (
	SQLSERVER_HOST         = "SQLSERVER_HOST"         //nolint:staticcheck
	SQLSERVER_PORT         = "SQLSERVER_PORT"         //nolint:staticcheck
	SQLSERVER_DATABASE     = "SQLSERVER_DATABASE"     //nolint:staticcheck
	SQLSERVER_USERNAME     = "SQLSERVER_USERNAME"     //nolint:staticcheck
	SQLSERVER_PASSWORD     = "SQLSERVER_PASSWORD"     //nolint:staticcheck,gosec
	SQLSERVER_ENCRYPT      = "SQLSERVER_ENCRYPT"      //nolint:staticcheck
	SQLSERVER_TRUST_SERVER = "SQLSERVER_TRUST_SERVER" //nolint:staticcheck
	SQLSERVER_APP_NAME     = "SQLSERVER_APP_NAME"     //nolint:staticcheck
	SQLSERVER_ENABLE       = "SQLSERVER_ENABLE"       //nolint:staticcheck
)

type SQLServer struct {
	Host        string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port        uint   `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Database    string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	Username    string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password    string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Encrypt     bool   `json:"encrypt" mapstructure:"encrypt" ini:"encrypt" yaml:"encrypt"`
	TrustServer bool   `json:"trust_server" mapstructure:"trust_server" ini:"trust_server" yaml:"trust_server"`
	AppName     string `json:"app_name" mapstructure:"app_name" ini:"app_name" yaml:"app_name"`
	Enable      bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*SQLServer) setDefault() {
	cv.SetDefault("sqlserver.host", "127.0.0.1")
	cv.SetDefault("sqlserver.port", 1433)
	cv.SetDefault("sqlserver.database", "")
	cv.SetDefault("sqlserver.username", "sa")
	cv.SetDefault("sqlserver.password", "")
	cv.SetDefault("sqlserver.encrypt", false)
	cv.SetDefault("sqlserver.trust_server", true)
	cv.SetDefault("sqlserver.app_name", consts.FrameworkName)
	cv.SetDefault("sqlserver.enable", false)
}
