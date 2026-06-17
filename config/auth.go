package config

import "time"

const (
	AUTH_NONE_EXPIRE_TOKEN             = "AUTH_NONE_EXPIRE_TOKEN"             //nolint:staticcheck,gosec
	AUTH_NONE_EXPIRE_USERNAME          = "AUTH_NONE_EXPIRE_USERNAME"          //nolint:staticcheck
	AUTH_NONE_EXPIRE_PASSWORD          = "AUTH_NONE_EXPIRE_PASSORD"           //nolint:staticcheck,gosec
	AUTH_BASE_AUTH_USERNAME            = "AUTH_BASE_AUTH_USERNAME"            //nolint:staticcheck
	AUTH_BASE_AUTH_PASSWORD            = "AUTH_BASE_AUTH_PASSWORD"            //nolint:staticcheck,gosec
	AUTH_ACCESS_TOKEN_EXPIRE_DURATION  = "AUTH_ACCESS_TOKEN_EXPIRE_DURATION"  //nolint:staticcheck,gosec
	AUTH_REFRESH_TOKEN_EXPIRE_DURATION = "AUTH_REFRESH_TOKEN_EXPIRE_DURATION" //nolint:staticcheck,gosec
	AUTH_RBAC_ENABLE                   = "AUTH_RBAC_ENABLE"                   //nolint:staticcheck
)

type Auth struct {
	NoneExpireToken            string        `json:"none_expire_token" mapstructure:"none_expire_token" ini:"none_expire_token" yaml:"none_expire_token"`
	NoneExpireUsername         string        `json:"none_expire_username" mapstructure:"none_expire_username" ini:"none_expire_username" yaml:"none_expire_username"`
	NoneExpirePassword         string        `json:"none_expire_passord" mapstructure:"none_expire_passord" ini:"none_expire_passord" yaml:"none_expire_passord"`
	BaseAuthUsername           string        `json:"base_auth_username" mapstructure:"base_auth_username" ini:"base_auth_username" yaml:"base_auth_username"`
	BaseAuthPassword           string        `json:"base_auth_password" mapstructure:"base_auth_password" ini:"base_auth_password" yaml:"base_auth_password"`
	AccessTokenExpireDuration  time.Duration `json:"access_token_expire_duration" mapstructure:"access_token_expire_duration" ini:"access_token_expire_duration" yaml:"access_token_expire_duration"`
	RefreshTokenExpireDuration time.Duration `json:"refresh_token_expire_duration" mapstructure:"refresh_token_expire_duration" ini:"refresh_token_expire_duration" yaml:"refresh_token_expire_duration"`

	RBACEnable bool `json:"rbac_enable" mapstructure:"rbac_enable" ini:"rbac_enable" yaml:"rbac_enable"`
}

func (*Auth) setDefault() {
	cv.SetDefault("auth.none_expire_token", noneExpireToken)
	cv.SetDefault("auth.none_expire_username", noneExpireUser)
	cv.SetDefault("auth.none_expire_passord", noneExpirePass)
	cv.SetDefault("auth.base_auth_username", baseAuthUsername)
	cv.SetDefault("auth.base_auth_password", baseAuthPassword)
	cv.SetDefault("auth.access_token_expire_duration", "2h")
	cv.SetDefault("auth.refresh_token_expire_duration", "168h")

	cv.SetDefault("auth.rbac_enable", false)
}
