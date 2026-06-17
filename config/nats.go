package config

import "time"

const (
	NATS_ADDRS       = "NATS_ADDRS"       //nolint:staticcheck
	NATS_CLIENT_NAME = "NATS_CLIENT_NAME" //nolint:staticcheck
	NATS_USERNAME    = "NATS_USERNAME"    //nolint:staticcheck
	NATS_PASSWORD    = "NATS_PASSWORD"    //nolint:staticcheck
	NATS_TOKEN       = "NATS_TOKEN"       //nolint:staticcheck
	NATS_CREDENTIALS = "NATS_CREDENTIALS" //nolint:staticcheck
	NATS_NKEY_FILE   = "NATS_NKEY_FILE"   //nolint:staticcheck

	NATS_MAX_RECONNECTS       = "NATS_MAX_RECONNECTS"       //nolint:staticcheck
	NATS_RECONNECT_WAIT       = "NATS_RECONNECT_WAIT"       //nolint:staticcheck
	NATS_RECONNECT_JITTER     = "NATS_RECONNECT_JITTER"     //nolint:staticcheck
	NATS_RECONNECT_JITTER_TLS = "NATS_RECONNECT_JITTER_TLS" //nolint:staticcheck

	NATS_CONNECT_TIMEOUT       = "NATS_CONNECT_TIMEOUT"       //nolint:staticcheck
	NATS_PING_INTERVAL         = "NATS_PING_INTERVAL"         //nolint:staticcheck
	NATS_MAX_PINGS_OUTSTANDING = "NATS_MAX_PINGS_OUTSTANDING" //nolint:staticcheck

	NATS_ENABLE_TLS           = "NATS_ENABLE_TLS"           //nolint:staticcheck
	NATS_CERT_FILE            = "NATS_CERT_FILE"            //nolint:staticcheck
	NATS_KEY_FILE             = "NATS_KEY_FILE"             //nolint:staticcheck
	NATS_CA_FILE              = "NATS_CA_FILE"              //nolint:staticcheck
	NATS_INSECURE_SKIP_VERIFY = "NATS_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	NATS_ENABLE = "NATS_ENABLE" //nolint:staticcheck
)

type Nats struct {
	Addrs           []string `json:"addrs" mapstructure:"addrs" ini:"addrs" yaml:"addrs"`
	ClientName      string   `json:"client_name" mapstructure:"client_name" ini:"client_name" yaml:"client_name"`
	Username        string   `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password        string   `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Token           string   `json:"token" mapstructure:"token" ini:"token" yaml:"token"`
	CredentialsFile string   `json:"credentials_file" mapstructure:"credentials_file" ini:"credentials_file" yaml:"credentials_file"`
	NKeyFile        string   `json:"nkey_file" mapstructure:"nkey_file" ini:"nkey_file" yaml:"nkey_file"`

	MaxReconnects      int           `json:"max_reconnects" mapstructure:"max_reconnects" ini:"max_reconnects" yaml:"max_reconnects"`
	ReconnectWait      time.Duration `json:"reconnect_wait" mapstructure:"reconnect_wait" ini:"reconnect_wait" yaml:"reconnect_wait"`
	ReconnectJitter    time.Duration `json:"reconnect_jitter" mapstructure:"reconnect_jitter" ini:"reconnect_jitter" yaml:"reconnect_jitter"`
	ReconnectJitterTLS time.Duration `json:"reconnect_jitter_tls" mapstructure:"reconnect_jitter_tls" ini:"reconnect_jitter_tls" yaml:"reconnect_jitter_tls"`

	ConnectTimeout      time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	PingInterval        time.Duration `json:"ping_interval" mapstructure:"ping_interval" ini:"ping_interval" yaml:"ping_interval"`
	MaxPingsOutstanding int           `json:"max_pings_outstanding" mapstructure:"max_pings_outstanding" ini:"max_pings_outstanding" yaml:"max_pings_outstanding"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Nats) setDefault() {
	cv.SetDefault("nats.addrs", []string{"nats://127.0.0.1:4222"})
	cv.SetDefault("nats.client_name", "")
	cv.SetDefault("nats.username", "")
	cv.SetDefault("nats.password", "")
	cv.SetDefault("nats.token", "")
	cv.SetDefault("nats.credentials_file", "")
	cv.SetDefault("nats.nkey_file", "")

	cv.SetDefault("nats.max_reconnects", 5)
	cv.SetDefault("nats.reconnect_wait", 1*time.Second)
	cv.SetDefault("nats.reconnect_jitter", 0)
	cv.SetDefault("nats.reconnect_jitter_tls", 0)

	cv.SetDefault("nats.connect_timeout", 2*time.Second)
	cv.SetDefault("nats.ping_interval", 2*time.Minute)
	cv.SetDefault("nats.max_pings_outstanding", 2)

	cv.SetDefault("nats.enable_tls", false)
	cv.SetDefault("nats.cert_file", "")
	cv.SetDefault("nats.key_file", "")
	cv.SetDefault("nats.ca_file", "")
	cv.SetDefault("nats.insecure_skip_verify", false)

	cv.SetDefault("nats.enable", false)
}
