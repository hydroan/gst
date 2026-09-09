package config

import (
	"time"

	"github.com/spf13/viper"
)

const (
	AUTH_BASE_AUTH_USERNAME            = "AUTH_BASE_AUTH_USERNAME"
	AUTH_BASE_AUTH_PASSWORD            = "AUTH_BASE_AUTH_PASSWORD"
	AUTH_ACCESS_TOKEN_EXPIRE_DURATION  = "AUTH_ACCESS_TOKEN_EXPIRE_DURATION"
	AUTH_REFRESH_TOKEN_EXPIRE_DURATION = "AUTH_REFRESH_TOKEN_EXPIRE_DURATION"
	AUTH_RBAC_ENABLED                  = "AUTH_RBAC_ENABLED"
)

type Auth struct {
	BaseAuthUsername           string        `json:"base_auth_username" mapstructure:"base_auth_username" ini:"base_auth_username" yaml:"base_auth_username"`
	BaseAuthPassword           string        `json:"base_auth_password" mapstructure:"base_auth_password" ini:"base_auth_password" yaml:"base_auth_password"`
	AccessTokenExpireDuration  time.Duration `json:"access_token_expire_duration" mapstructure:"access_token_expire_duration" ini:"access_token_expire_duration" yaml:"access_token_expire_duration"`
	RefreshTokenExpireDuration time.Duration `json:"refresh_token_expire_duration" mapstructure:"refresh_token_expire_duration" ini:"refresh_token_expire_duration" yaml:"refresh_token_expire_duration"`

	RBACEnabled bool `json:"rbac_enabled" mapstructure:"rbac_enabled" ini:"rbac_enabled" yaml:"rbac_enabled"`
}

func (*Auth) setDefault(v *viper.Viper) {
	v.SetDefault("auth.base_auth_username", baseAuthUsername)
	v.SetDefault("auth.base_auth_password", baseAuthPassword)
	v.SetDefault("auth.access_token_expire_duration", "2h")
	v.SetDefault("auth.refresh_token_expire_duration", "168h")

	v.SetDefault("auth.rbac_enabled", false)
}
