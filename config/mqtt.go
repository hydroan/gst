package config

import "time"

const (
	MQTT_ADDR                 = "MQTT_ADDR"                 //nolint:staticcheck
	MQTT_USERNAME             = "MQTT_USERNAME"             //nolint:staticcheck
	MQTT_PASSWORD             = "MQTT_PASSWORD"             //nolint:staticcheck
	MQTT_CLIENT_PREFIX        = "MQTT_CLIENT_PREFIX"        //nolint:staticcheck
	MQTT_CONNECT_TIMEOUT      = "MQTT_CONNECT_TIMEOUT"      //nolint:staticcheck
	MQTT_KEEPALIVE            = "MQTT_KEEPALIVE"            //nolint:staticcheck
	MQTT_CLEAN_SESSION        = "MQTT_CLEAN_SESSION"        //nolint:staticcheck
	MQTT_AUTO_RECONNECT       = "MQTT_AUTO_RECONNECT"       //nolint:staticcheck
	MQTT_USE_TLS              = "MQTT_USE_TLS"              //nolint:staticcheck
	MQTT_CERT_FILE            = "MQTT_CERT_FILE"            //nolint:staticcheck
	MQTT_KEY_FILE             = "MQTT_KEY_FILE"             //nolint:staticcheck
	MQTT_INSECURE_SKIP_VERIFY = "MQTT_INSECURE_SKIP_VERIFY" //nolint:staticcheck
	MQTT_ENABLE               = "MQTT_ENABLE"               //nolint:staticcheck
)

type Mqtt struct {
	Addr               string        `json:"addr" mapstructure:"addr" ini:"addr" yaml:"addr"`
	Username           string        `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password           string        `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	ClientPrefix       string        `json:"client_prefix" mapstructure:"client_prefix" ini:"client_prefix" yaml:"client_prefix"`
	ConnectTimeout     time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	Keepalive          time.Duration `json:"keepalive" mapstructure:"keepalive" ini:"keepalive" yaml:"keepalive"`
	CleanSession       bool          `json:"clean_session" mapstructure:"clean_session" ini:"clean_session" yaml:"clean_session"`
	AutoReconnect      bool          `json:"auto_reconnect" mapstructure:"auto_reconnect" ini:"auto_reconnect" yaml:"auto_reconnect"`
	UseTLS             bool          `json:"use_tls" mapstructure:"use_tls" ini:"use_tls" yaml:"use_tls"`
	CertFile           string        `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string        `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	InsecureSkipVerify bool          `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`
	Enable             bool          `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Mqtt) setDefault() {
	cv.SetDefault("mqtt.addr", "127.0.0.1:1883")
	cv.SetDefault("mqtt.username", "")
	cv.SetDefault("mqtt.password", "")
	cv.SetDefault("mqtt.client_prefix", "")
	cv.SetDefault("mqtt.connect_timeout", 10*time.Second)
	cv.SetDefault("mqtt.keepalive", 1*time.Minute)
	cv.SetDefault("mqtt.clean_session", true)
	cv.SetDefault("mqtt.auto_reconnect", true)
	cv.SetDefault("mqtt.use_tls", false)
	cv.SetDefault("mqtt.cert_file", "")
	cv.SetDefault("mqtt.key_file", "")
	cv.SetDefault("mqtt.insecure_skip_verify", true)
	cv.SetDefault("mqtt.enable", false)
}
