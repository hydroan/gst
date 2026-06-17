package config

import "time"

type FeishuAppType string

const (
	FeishuAppTypeSelfBuilt   FeishuAppType = "SelfBuilt"
	FeishuAppTypeMarketplace FeishuAppType = "Marketplace"
)

const (
	FEISHU_APP_ID     = "FEISHU_APP_ID"     //nolint:staticcheck
	FEISHU_APP_SECRET = "FEISHU_APP_SECRET" //nolint:staticcheck,gosec
	FEISHU_APP_TYPE   = "FEISHU_APP_TYPE"   //nolint:staticcheck

	FEISHU_DISABLE_TOKEN_CACHE = "FEISHU_DISABLE_TOKEN_CACHE" //nolint:staticcheck,gosec
	FEISHU_REQUEST_TIMEOUT     = "FEISHU_REQUEST_TIMEOUT"     //nolint:staticcheck

	FEISHU_ENABLE_TLS           = "FEISHU_ENABLE_TLS"           //nolint:staticcheck
	FEISHU_CERT_FILE            = "FEISHU_CERT_FILE"            //nolint:staticcheck
	FEISHU_KEY_FILE             = "FEISHU_KEY_FILE"             //nolint:staticcheck
	FEISHU_CA_FILE              = "FEISHU_CA_FILE"              //nolint:staticcheck
	FEISHU_INSECURE_SKIP_VERIFY = "FEISHU_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	FEISHU_ENABLE = "FEISHU_ENABLE" //nolint:staticcheck
)

type Feishu struct {
	AppID     string        `json:"app_id" mapstructure:"app_id" ini:"app_id" yaml:"app_id"`
	AppSecret string        `json:"app_secret" mapstructure:"app_secret" ini:"app_secret" yaml:"app_secret"`
	AppType   FeishuAppType `json:"app_type" mapstructure:"app_type" ini:"app_type" yaml:"app_type"`

	DisableTokenCache bool          `json:"disable_token_cache" mapstructure:"disable_token_cache" ini:"disable_token_cache" yaml:"disable_token_cache"`
	RequestTimeout    time.Duration `json:"request_timeout" mapstructure:"request_timeout" ini:"request_timeout" yaml:"request_timeout"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Feishu) setDefault() {
	cv.SetDefault("feishu.app_id", "")
	cv.SetDefault("feishu.app_secret", "")
	cv.SetDefault("feishu.app_type", "INTERNAL")

	cv.SetDefault("feishu.disable_token_cache", false)
	cv.SetDefault("feishu.request_timeout", 15*time.Second)

	cv.SetDefault("feishu.enable_tls", false)
	cv.SetDefault("feishu.cert_file", "")
	cv.SetDefault("feishu.key_file", "")
	cv.SetDefault("feishu.ca_file", "")
	cv.SetDefault("feishu.insecure_skip_verify", false)

	cv.SetDefault("feishu.enable", false)
}
