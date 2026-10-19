package config

import (
	"time"

	"github.com/spf13/viper"
)

// ConsumeFromWhere names where a new consumer group starts reading a queue.
type ConsumeFromWhere string

const (
	ConsumeFromWhereLastOffset  ConsumeFromWhere = "last_offset"  // consume from the last committed offset
	ConsumeFromWhereFirstOffset ConsumeFromWhere = "first_offset" // consume from the beginning of the queue
	ConsumeFromWhereTimestamp   ConsumeFromWhere = "timestamp"    // consume from the given timestamp
)

// UnmarshalText reads text as one of the starting points, in any letter case;
// any other text is a configuration error naming the key.
func (w *ConsumeFromWhere) UnmarshalText(text []byte) (err error) {
	*w, err = oneOf(text, ConsumeFromWhereLastOffset, ConsumeFromWhereFirstOffset, ConsumeFromWhereTimestamp)
	return err
}

const (
	ROCKETMQ_NAME_SERVER_ADDRS = "ROCKETMQ_NAME_SERVER_ADDRS"
	ROCKETMQ_ACCESS_KEY        = "ROCKETMQ_ACCESS_KEY"
	ROCKETMQ_SECRET_KEY        = "ROCKETMQ_SECRET_KEY"
	ROCKETMQ_NAMESPACE         = "ROCKETMQ_NAMESPACE"
	ROCKETMQ_GROUP_NAME        = "ROCKETMQ_GROUP_NAME"
	ROCKETMQ_INSTANCE_NAME     = "ROCKETMQ_INSTANCE_NAME"

	ROCKETMQ_NUM_RETRIES         = "ROCKETMQ_NUM_RETRIES"
	ROCKETMQ_SEND_MSG_TIMEOUT    = "ROCKETMQ_SEND_MSG_TIMEOUT"
	ROCKETMQ_VIP_CHANNEL_ENABLED = "ROCKETMQ_VIP_CHANNEL_ENABLED"

	ROCKETMQ_CONSUME_ORDERLY                = "ROCKETMQ_CONSUME_ORDERLY"
	ROCKETMQ_MAX_RECONSUME_TIMES            = "ROCKETMQ_MAX_RECONSUME_TIMES"
	ROCKETMQ_AUTO_COMMIT                    = "ROCKETMQ_AUTO_COMMIT"
	ROCKETMQ_CONSUME_CONCURRENTLY_MAX_SPAN  = "ROCKETMQ_CONSUME_CONCURRENTLY_MAX_SPAN"
	ROCKETMQ_CONSUME_MESSAGE_BATCH_MAX_SIZE = "ROCKETMQ_CONSUME_MESSAGE_BATCH_MAX_SIZE"

	ROCKETMQ_CONSUME_FROM_WHERE = "ROCKETMQ_CONSUME_FROM_WHERE"
	ROCKETMQ_CONSUME_TIMESTAMP  = "ROCKETMQ_CONSUME_TIMESTAMP"

	ROCKETMQ_TRACE_ENABLED = "ROCKETMQ_TRACE_ENABLED"

	ROCKETMQ_ENABLED = "ROCKETMQ_ENABLED"
)

type RocketMQ struct {
	NameServerAddrs []string `json:"name_server_addrs" mapstructure:"name_server_addrs" ini:"name_server_addrs" yaml:"name_server_addrs"`
	AccessKey       string   `json:"access_key" mapstructure:"access_key" ini:"access_key" yaml:"access_key"`
	SecretKey       string   `json:"secret_key" mapstructure:"secret_key" ini:"secret_key" yaml:"secret_key"`
	Namespace       string   `json:"namespace" mapstructure:"namespace" ini:"namespace" yaml:"namespace"`
	GroupName       string   `json:"group_name" mapstructure:"group_name" ini:"group_name" yaml:"group_name"`
	// InstanceName names the process's clients to the broker. Empty leaves it
	// to the client library, which names every process apart; a name set here
	// has to differ between processes consuming in one group on one machine,
	// or the broker counts them as one consumer.
	InstanceName string `json:"instance_name" mapstructure:"instance_name" ini:"instance_name" yaml:"instance_name"`

	NumRetries        int           `json:"num_retries" mapstructure:"num_retries" ini:"num_retries" yaml:"num_retries"`
	SendMsgTimeout    time.Duration `json:"send_msg_timeout" mapstructure:"send_msg_timeout" ini:"send_msg_timeout" yaml:"send_msg_timeout"`
	VipChannelEnabled bool          `json:"vip_channel_enabled" mapstructure:"vip_channel_enabled" ini:"vip_channel_enabled" yaml:"vip_channel_enabled"`

	ConsumeOrderly             bool  `json:"consume_orderly" mapstructure:"consume_orderly" ini:"consume_orderly" yaml:"consume_orderly"`
	MaxReconsumeTime           int32 `json:"max_reconsume_times" mapstructure:"max_reconsume_times" ini:"max_reconsume_times" yaml:"max_reconsume_times"`
	AutoCommit                 bool  `json:"auto_commit" mapstructure:"auto_commit" ini:"auto_commit" yaml:"auto_commit"`
	ConsumeConcurrentlyMaxSpan int   `json:"consume_concurrently_max_span" mapstructure:"consume_concurrently_max_span" ini:"consume_concurrently_max_span" yaml:"consume_concurrently_max_span"`
	ConsumeMessageBatchMaxSize int   `json:"consume_message_batch_max_size" mapstructure:"consume_message_batch_max_size" ini:"consume_message_batch_max_size" yaml:"consume_message_batch_max_size"`

	ConsumeFromWhere ConsumeFromWhere `json:"consume_from_where" mapstructure:"consume_from_where" ini:"consume_from_where" yaml:"consume_from_where"`
	ConsumeTimestamp string           `json:"consume_timestamp" mapstructure:"consume_timestamp" ini:"consume_timestamp" yaml:"consume_timestamp"`

	TraceEnabled bool `json:"trace_enabled" mapstructure:"trace_enabled" ini:"trace_enabled" yaml:"trace_enabled"`

	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*RocketMQ) setDefaults(v *viper.Viper) {
	v.SetDefault("rocketmq.name_server_addrs", []string{"127.0.0.1:9876"})
	v.SetDefault("rocketmq.access_key", "")
	v.SetDefault("rocketmq.secret_key", "")
	v.SetDefault("rocketmq.namespace", "")
	v.SetDefault("rocketmq.group_name", "DEFAULT_PRODUCER")
	v.SetDefault("rocketmq.instance_name", "")

	v.SetDefault("rocketmq.num_retries", 2)
	v.SetDefault("rocketmq.send_msg_timeout", 3*time.Second)
	v.SetDefault("rocketmq.vip_channel_enabled", false)

	v.SetDefault("rocketmq.consume_orderly", false)
	v.SetDefault("rocketmq.max_reconsume_times", 16)
	v.SetDefault("rocketmq.auto_commit", true)
	v.SetDefault("rocketmq.consume_concurrently_max_span", 2000)
	v.SetDefault("rocketmq.consume_message_batch_max_size", 1)

	v.SetDefault("rocketmq.consume_from_where", ConsumeFromWhereLastOffset)
	v.SetDefault("rocketmq.consume_timestamp", "")

	v.SetDefault("rocketmq.trace_enabled", false)

	v.SetDefault("rocketmq.enabled", false)
}
