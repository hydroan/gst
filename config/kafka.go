package config

import "time"

const (
	KAFKA_BROKERS   = "KAFKA_BROKERS"   //nolint:staticcheck
	KAFKA_CLIENT_ID = "KAFKA_CLIENT_ID" //nolint:staticcheck
	KAFKA_VERSION   = "KAFKA_VERSION"   //nolint:staticcheck

	KAFKA_SASL_ENABLE    = "KAFKA_SASL_ENABLE"    //nolint:staticcheck
	KAFKA_SASL_MECHANISM = "KAFKA_SASL_MECHANISM" //nolint:staticcheck
	KAFKA_SASL_USERNAME  = "KAFKA_SASL_USERNAME"  //nolint:staticcheck
	KAFKA_SASL_PASSWORD  = "KAFKA_SASL_PASSWORD"  //nolint:staticcheck,gosec

	KAFKA_ENABLE_TLS           = "KAFKA_ENABLE_TLS"           //nolint:staticcheck
	KAFKA_CERT_FILE            = "KAFKA_CERT_FILE"            //nolint:staticcheck
	KAFKA_KEY_FILE             = "KAFKA_KEY_FILE"             //nolint:staticcheck
	KAFKA_CA_FILE              = "KAFKA_CA_FILE"              //nolint:staticcheck
	KAFKA_INSECURE_SKIP_VERIFY = "KAFKA_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	KAFKA_PRODUCER_REQUIRED_ACKS     = "KAFKA_PRODUCER_REQUIRED_ACKS"     //nolint:staticcheck
	KAFKA_PRODUCER_RETRIES           = "KAFKA_PRODUCER_RETRIES"           //nolint:staticcheck
	KAFKA_PRODUCER_COMPRESSION       = "KAFKA_PRODUCER_COMPRESSION"       //nolint:staticcheck
	KAFKA_PRODUCER_MAX_MESSAGE_BYTES = "KAFKA_PRODUCER_MAX_MESSAGE_BYTES" //nolint:staticcheck

	KAFKA_CONSUMER_GROUP              = "KAFKA_CONSUMER_GROUP"              //nolint:staticcheck
	KAFKA_CONSUMER_REBALANCE_STRATEGY = "KAFKA_CONSUMER_REBALANCE_STRATEGY" //nolint:staticcheck
	KAFKA_CONSUMER_OFFSET             = "KAFKA_CONSUMER_OFFSET"             //nolint:staticcheck

	KAFKA_TIMEOUT_DIAL  = "KAFKA_TIMEOUT_DIAL"  //nolint:staticcheck
	KAFKA_TIMEOUT_READ  = "KAFKA_TIMEOUT_READ"  //nolint:staticcheck
	KAFKA_TIMEOUT_WRITE = "KAFKA_TIMEOUT_WRITE" //nolint:staticcheck

	KAFKA_ENABLE = "KAFKA_ENABLE" //nolint:staticcheck
)

// SASL defines SASL authentication parameters
type SASL struct {
	Enable    bool   `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
	Mechanism string `json:"mechanism" mapstructure:"mechanism" ini:"mechanism" yaml:"mechanism"`
	Username  string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password  string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
}

// Producer defines Kafka producer specific configuration
type Producer struct {
	RequiredAcks    int    `json:"required_acks" mapstructure:"required_acks" ini:"required_acks" yaml:"required_acks"`
	Retries         int    `json:"retries" mapstructure:"retries" ini:"retries" yaml:"retries"`
	Compression     string `json:"compression" mapstructure:"compression" ini:"compression" yaml:"compression"`
	MaxMessageBytes int    `json:"max_message_bytes" mapstructure:"max_message_bytes" ini:"max_message_bytes" yaml:"max_message_bytes"`
}

// Consumer defines Kafka consumer specific configuration
type Consumer struct {
	Group             string `json:"group" mapstructure:"group" ini:"group" yaml:"group"`
	RebalanceStrategy string `json:"rebalance_strategy" mapstructure:"rebalance_strategy" ini:"rebalance_strategy" yaml:"rebalance_strategy"`
	Offset            string `json:"offset" mapstructure:"offset" ini:"offset" yaml:"offset"`
}

// Timeout defines connection timeouts
type Timeout struct {
	Dial  time.Duration `json:"dial" mapstructure:"dial" ini:"dial" yaml:"dial"`
	Read  time.Duration `json:"read" mapstructure:"read" ini:"read" yaml:"read"`
	Write time.Duration `json:"write" mapstructure:"write" ini:"write" yaml:"write"`
}

type Kafka struct {
	Brokers  []string `json:"brokers" mapstructure:"brokers" ini:"brokers" yaml:"brokers"`
	ClientID string   `json:"client_id" mapstructure:"client_id" ini:"client_id" yaml:"client_id"`
	Version  string   `json:"version" mapstructure:"version" ini:"version" yaml:"version"`

	SASL SASL `json:"sasl" mapstructure:"sasl" ini:"sasl" yaml:"sasl"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Producer Producer `json:"producer" mapstructure:"producer" ini:"producer" yaml:"producer"`
	Consumer Consumer `json:"consumer" mapstructure:"consumer" ini:"consumer" yaml:"consumer"`
	Timeout  Timeout  `json:"timeout" mapstructure:"timeout" ini:"timeout" yaml:"timeout"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Kafka) setDefault() {
	cv.SetDefault("kafka.brokers", []string{"localhost:9092"})
	cv.SetDefault("kafka.client_id", "sarama-client")
	cv.SetDefault("kafka.version", "")

	cv.SetDefault("kafka.sasl.enable", false)
	cv.SetDefault("kafka.sasl.mechanism", "PLAIN")
	cv.SetDefault("kafka.sasl.username", "")
	cv.SetDefault("kafka.sasl.password", "")

	cv.SetDefault("kafka.enable_tls", false)
	cv.SetDefault("kafka.cert_file", "")
	cv.SetDefault("kafka.key_file", "")
	cv.SetDefault("kafka.ca_file", "")
	cv.SetDefault("kafka.insecure_skip_verify", false)

	cv.SetDefault("kafka.producer.required_acks", 1)
	cv.SetDefault("kafka.producer.retries", 3)
	cv.SetDefault("kafka.producer.compression", "none")
	cv.SetDefault("kafka.producer.max_message_bytes", 1000000)

	cv.SetDefault("kafka.consumer.group", "")
	cv.SetDefault("kafka.consumer.rebalance_strategy", "range")
	cv.SetDefault("kafka.consumer.offset", "newest")

	cv.SetDefault("kafka.timeout.dial", 10*time.Second)
	cv.SetDefault("kafka.timeout.read", 30*time.Second)
	cv.SetDefault("kafka.timeout.write", 30*time.Second)

	cv.SetDefault("kafka.enable", false)
}
