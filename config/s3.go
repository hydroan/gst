package config

import "github.com/spf13/viper"

const (
	S3_ENDPOINT          = "S3_ENDPOINT"          //nolint:staticcheck
	S3_REGION            = "S3_REGION"            //nolint:staticcheck
	S3_ACCESS_KEY_ID     = "S3_ACCESS_KEY_ID"     //nolint:staticcheck
	S3_SECRET_ACCESS_KEY = "S3_SECRET_ACCESS_KEY" //nolint:staticcheck
	S3_BUCKET            = "S3_BUCKET"            //nolint:staticcheck
	S3_USE_SSL           = "S3_USE_SSL"           //nolint:staticcheck
	S3_ENABLED           = "S3_ENABLED"           //nolint:staticcheck
)

type S3 struct {
	Endpoint        string `json:"endpoint" mapstructure:"endpoint" ini:"endpoint" yaml:"endpoint"`
	Region          string `json:"region" mapstructure:"region" ini:"region" yaml:"region"`
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" ini:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" ini:"secret_access_key" yaml:"secret_access_key"`
	Bucket          string `json:"bucket" mapstructure:"bucket" ini:"bucket" yaml:"bucket"`
	UseSsl          bool   `json:"use_ssl" mapstructure:"use_ssl" ini:"use_ssl" yaml:"use_ssl"`
	Enabled         bool   `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*S3) setDefault(v *viper.Viper) {
	v.SetDefault("s3.endpoint", "")
	v.SetDefault("s3.region", "")
	v.SetDefault("s3.access_key_id", "")
	v.SetDefault("s3.secret_access_key", "")
	v.SetDefault("s3.bucket", "")
	v.SetDefault("s3.use_ssl", false)
	v.SetDefault("s3.enabled", false)
}
