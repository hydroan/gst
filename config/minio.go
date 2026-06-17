package config

import (
	"time"
)

const (
	MINIO_ENDPOINT      = "MINIO_ENDPOINT"      //nolint:staticcheck
	MINIO_ACCESS_KEY    = "MINIO_ACCESS_KEY"    //nolint:staticcheck
	MINIO_SECRET_KEY    = "MINIO_SECRET_KEY"    //nolint:staticcheck
	MINIO_BUCKET        = "MINIO_BUCKET"        //nolint:staticcheck
	MINIO_LOCATION      = "MINIO_LOCATION"      //nolint:staticcheck
	MINIO_SECURE        = "MINIO_SECURE"        //nolint:staticcheck
	MINIO_REGION        = "MINIO_REGION"        //nolint:staticcheck
	MINIO_TIMEOUT       = "MINIO_TIMEOUT"       //nolint:staticcheck
	MINIO_PART_SIZE     = "MINIO_PART_SIZE"     //nolint:staticcheck
	MINIO_CONCURRENCY   = "MINIO_CONCURRENCY"   //nolint:staticcheck
	MINIO_COMPRESS      = "MINIO_COMPRESS"      //nolint:staticcheck
	MINIO_TRACE         = "MINIO_TRACE"         //nolint:staticcheck
	MINIO_SESSION_TOKEN = "MINIO_SESSION_TOKEN" //nolint:staticcheck
	MINIO_USE_IAM       = "MINIO_USE_IAM"       //nolint:staticcheck
	MINIO_USE_STS       = "MINIO_USE_STS"       //nolint:staticcheck
	MINIO_IAM_ENDPOINT  = "MINIO_IAM_ENDPOINT"  //nolint:staticcheck
	MINIO_STS_ENDPOINT  = "MINIO_STS_ENDPOINT"  //nolint:staticcheck

	MINIO_ENABLE_TLS           = "MINIO_ENABLE_TLS"           //nolint:staticcheck
	MINIO_CERT_FILE            = "MINIO_CERT_FILE"            //nolint:staticcheck
	MINIO_KEY_FILE             = "MINIO_KEY_FILE"             //nolint:staticcheck
	MINIO_CA_FILE              = "MINIO_CA_FILE"              //nolint:staticcheck
	MINIO_INSECURE_SKIP_VERIFY = "MINIO_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	MINIO_ENABLE = "MINIO_ENABLE" //nolint:staticcheck
)

type Minio struct {
	Endpoint     string        `json:"endpoint" mapstructure:"endpoint" ini:"endpoint" yaml:"endpoint"`
	AccessKey    string        `json:"access_key" mapstructure:"access_key" ini:"access_key" yaml:"access_key"`
	SecretKey    string        `json:"secret_key" mapstructure:"secret_key" ini:"secret_key" yaml:"secret_key"`
	Bucket       string        `json:"bucket" mapstructure:"bucket" ini:"bucket" yaml:"bucket"`
	Location     string        `json:"location" mapstructure:"location" ini:"location" yaml:"location"`
	Secure       bool          `json:"secure" mapstructure:"secure" ini:"secure" yaml:"secure"`
	Region       string        `json:"region" mapstructure:"region" ini:"region" yaml:"region"`
	Timeout      time.Duration `json:"timeout" mapstructure:"timeout" ini:"timeout" yaml:"timeout"`
	PartSize     int64         `json:"part_size" mapstructure:"part_size" ini:"part_size" yaml:"part_size"`
	Concurrency  int           `json:"concurrency" mapstructure:"concurrency" ini:"concurrency" yaml:"concurrency"`
	Compress     bool          `json:"compress" mapstructure:"compress" ini:"compress" yaml:"compress"`
	Trace        bool          `json:"trace" mapstructure:"trace" ini:"trace" yaml:"trace"`
	SessionToken string        `json:"session_token" mapstructure:"session_token" ini:"session_token" yaml:"session_token"`
	UseIAM       bool          `json:"use_iam" mapstructure:"use_iam" ini:"use_iam" yaml:"use_iam"`
	UseSTS       bool          `json:"use_sts" mapstructure:"use_sts" ini:"use_sts" yaml:"use_sts"`
	IAMEndpoint  string        `json:"iam_endpoint" mapstructure:"iam_endpoint" ini:"iam_endpoint" yaml:"iam_endpoint"`
	STSEndpoint  string        `json:"sts_endpoint" mapstructure:"sts_endpoint" ini:"sts_endpoint" yaml:"sts_endpoint"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Minio) setDefault() {
	cv.SetDefault("minio.endpoint", "localhost:9000")
	cv.SetDefault("minio.access_key", "")
	cv.SetDefault("minio.secret_key", "")
	cv.SetDefault("minio.bucket", "")
	cv.SetDefault("minio.location", "")
	cv.SetDefault("minio.secure", false)
	cv.SetDefault("minio.region", "")
	cv.SetDefault("minio.timeout", 10*time.Second)
	cv.SetDefault("minio.part_size", 0) // 5MB part size
	cv.SetDefault("minio.concurrency", 0)
	cv.SetDefault("minio.compress", false)
	cv.SetDefault("minio.trace", false)
	cv.SetDefault("minio.session_token", "")
	cv.SetDefault("minio.use_iam", false)
	cv.SetDefault("minio.use_sts", false)
	cv.SetDefault("minio.iam_endpoint", "")
	cv.SetDefault("minio.sts_endpoint", "")

	cv.SetDefault("minio.enable_tls", false)
	cv.SetDefault("minio.cert_file", "")
	cv.SetDefault("minio.key_file", "")
	cv.SetDefault("minio.ca_file", "")
	cv.SetDefault("minio.insecure_skip_verify", false)

	cv.SetDefault("minio.enable", false)
}
