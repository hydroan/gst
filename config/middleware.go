package config

import "github.com/spf13/viper"

const (
	MIDDLEWARE_JWT_AUTH_ENABLED    = "MIDDLEWARE_JWT_AUTH_ENABLED"    //nolint:staticcheck
	MIDDLEWARE_AUTHZ_ENABLED       = "MIDDLEWARE_AUTHZ_ENABLED"       //nolint:staticcheck
	MIDDLEWARE_IAM_SESSION_ENABLED = "MIDDLEWARE_IAM_SESSION_ENABLED" //nolint:staticcheck
)

type Middleware struct {
	JWTAuthEnabled    bool `json:"jwt_auth_enabled" mapstructure:"jwt_auth_enabled" ini:"jwt_auth_enabled" yaml:"jwt_auth_enabled"`
	AuthzEnabled      bool `json:"authz_enabled" mapstructure:"authz_enabled" ini:"authz_enabled" yaml:"authz_enabled"`
	IAMSessionEnabled bool `json:"iam_session_enabled" mapstructure:"iam_session_enabled" ini:"iam_session_enabled" yaml:"iam_session_enabled"`
}

func (*Middleware) setDefault(v *viper.Viper) {
	v.SetDefault("middleware.jwt_auth_enabled", false)
	v.SetDefault("middleware.authz_enabled", false)
	v.SetDefault("middleware.iam_session_enabled", false)
}
