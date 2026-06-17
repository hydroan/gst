package config

import (
	"time"
)

const (
	ETCD_ENDPOINTS              = "ETCD_ENDPOINTS"              //nolint:staticcheck
	ETCD_DIAL_TIMEOUT           = "ETCD_DIAL_TIMEOUT"           //nolint:staticcheck
	ETCD_USERNAME               = "ETCD_USERNAME"               //nolint:staticcheck
	ETCD_PASSWORD               = "ETCD_PASSWORD"               //nolint:staticcheck
	ETCD_AUTO_SYNC              = "ETCD_AUTO_SYNC"              //nolint:staticcheck
	ETCD_AUTO_SYNC_INTERVAL     = "ETCD_AUTO_SYNC_INTERVAL"     //nolint:staticcheck
	ETCD_KEEPALIVE_TIME         = "ETCD_KEEPALIVE_TIME"         //nolint:staticcheck
	ETCD_KEEPALIVE_TIMEOUT      = "ETCD_KEEPALIVE_TIMEOUT"      //nolint:staticcheck
	ETCD_MAX_CALL_SEND_MSG_SIZE = "ETCD_MAX_CALL_SEND_MSG_SIZE" //nolint:staticcheck
	ETCD_MAX_CALL_RECV_MSG_SIZE = "ETCD_MAX_CALL_RECV_MSG_SIZE" //nolint:staticcheck
	ETCD_PERMIT_WITHOUT_STREAM  = "ETCD_PERMIT_WITHOUT_STREAM"  //nolint:staticcheck
	ETCD_REJECT_OLD_CLUSTER     = "ETCD_REJECT_OLD_CLUSTER"     //nolint:staticcheck

	ETCD_ENABLE_TLS           = "ETCD_ENABLE_TLS"           //nolint:staticcheck
	ETCD_CERT_FILE            = "ETCD_CERT_FILE"            //nolint:staticcheck
	ETCD_KEY_FILE             = "ETCD_KEY_FILE"             //nolint:staticcheck
	ETCD_CA_FILE              = "ETCD_CA_FILE"              //nolint:staticcheck
	ETCD_INSECURE_SKIP_VERIFY = "ETCD_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	ETCD_ENABLE = "ETCD_ENABLE" //nolint:staticcheck
)

// Etcd 配置结构
type Etcd struct {
	Endpoints           []string      `json:"endpoints" mapstructure:"endpoints" ini:"endpoints" yaml:"endpoints"`
	DialTimeout         time.Duration `json:"dial_timeout" mapstructure:"dial_timeout" ini:"dial_timeout" yaml:"dial_timeout"`
	Username            string        `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password            string        `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	AutoSync            bool          `json:"auto_sync" mapstructure:"auto_sync" ini:"auto_sync" yaml:"auto_sync"`
	AutoSyncInterval    time.Duration `json:"auto_sync_interval" mapstructure:"auto_sync_interval" ini:"auto_sync_interval" yaml:"auto_sync_interval"`
	KeepAliveTime       time.Duration `json:"keepalive_time" mapstructure:"keepalive_time" ini:"keepalive_time" yaml:"keepalive_time"`
	KeepAliveTimeout    time.Duration `json:"keepalive_timeout" mapstructure:"keepalive_timeout" ini:"keepalive_timeout" yaml:"keepalive_timeout"`
	MaxCallSendMsgSize  int           `json:"max_call_send_msg_size" mapstructure:"max_call_send_msg_size" ini:"max_call_send_msg_size" yaml:"max_call_send_msg_size"`
	MaxCallRecvMsgSize  int           `json:"max_call_recv_msg_size" mapstructure:"max_call_recv_msg_size" ini:"max_call_recv_msg_size" yaml:"max_call_recv_msg_size"`
	PermitWithoutStream bool          `json:"permit_without_stream" mapstructure:"permit_without_stream" ini:"permit_without_stream" yaml:"permit_without_stream"`
	RejectOldCluster    bool          `json:"reject_old_cluster" mapstructure:"reject_old_cluster" ini:"reject_old_cluster" yaml:"reject_old_cluster"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Etcd) setDefault() {
	cv.SetDefault("etcd.endpoints", []string{"127.0.0.1:2379"})
	cv.SetDefault("etcd.dial_timeout", 5*time.Second)
	cv.SetDefault("etcd.username", "")
	cv.SetDefault("etcd.password", "")
	cv.SetDefault("etcd.auto_sync", false)
	cv.SetDefault("etcd.auto_sync_interval", 0)
	cv.SetDefault("etcd.keepalive_time", 0)
	cv.SetDefault("etcd.keepalive_timeout", 0)
	cv.SetDefault("etcd.max_call_send_msg_size", 0)
	cv.SetDefault("etcd.max_call_recv_msg_size", 0)
	cv.SetDefault("etcd.permit_without_stream", false)
	cv.SetDefault("etcd.reject_old_cluster", false)

	cv.SetDefault("etcd.enable_tls", false)
	cv.SetDefault("etcd.cert_file", "")
	cv.SetDefault("etcd.key_file", "")
	cv.SetDefault("etcd.ca_file", "")
	cv.SetDefault("etcd.insecure_skip_verify", false)

	cv.SetDefault("etcd.enable", false)
}
