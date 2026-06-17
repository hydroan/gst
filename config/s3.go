package config

const (
	S3_ENDPOINT          = "S3_ENDPOINT"          //nolint:staticcheck
	S3_REGION            = "S3_REGION"            //nolint:staticcheck
	S3_ACCESS_KEY_ID     = "S3_ACCESS_KEY_ID"     //nolint:staticcheck
	S3_SECRET_ACCESS_KEY = "S3_SECRET_ACCESS_KEY" //nolint:staticcheck
	S3_BUCKET            = "S3_BUCKET"            //nolint:staticcheck
	S3_USE_SSL           = "S3_USE_SSL"           //nolint:staticcheck
	S3_ENABLE            = "S3_ENABLE"            //nolint:staticcheck
)

type S3 struct {
	Endpoint        string `json:"endpoint" mapstructure:"endpoint" ini:"endpoint" yaml:"endpoint"`
	Region          string `json:"region" mapstructure:"region" ini:"region" yaml:"region"`
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" ini:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" ini:"secret_access_key" yaml:"secret_access_key"`
	Bucket          string `json:"bucket" mapstructure:"bucket" ini:"bucket" yaml:"bucket"`
	UseSsl          bool   `json:"use_ssl" mapstructure:"use_ssl" ini:"use_ssl" yaml:"use_ssl"`
	Enable          bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*S3) setDefault() {
	cv.SetDefault("s3.endpoint", "")
	cv.SetDefault("s3.region", "")
	cv.SetDefault("s3.access_key_id", "")
	cv.SetDefault("s3.secret_access_key", "")
	cv.SetDefault("s3.bucket", "")
	cv.SetDefault("s3.use_ssl", false)
	cv.SetDefault("s3.enable", false)
}
