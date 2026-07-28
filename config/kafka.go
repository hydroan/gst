package config

const (
	KAFKA_BROKERS   = "KAFKA_BROKERS"   //nolint:staticcheck
	KAFKA_CLIENT_ID = "KAFKA_CLIENT_ID" //nolint:staticcheck

	KAFKA_SASL_ENABLED   = "KAFKA_SASL_ENABLED"   //nolint:staticcheck
	KAFKA_SASL_MECHANISM = "KAFKA_SASL_MECHANISM" //nolint:staticcheck
	KAFKA_SASL_USERNAME  = "KAFKA_SASL_USERNAME"  //nolint:staticcheck
	KAFKA_SASL_PASSWORD  = "KAFKA_SASL_PASSWORD"  //nolint:staticcheck,gosec

	KAFKA_TLS_ENABLED          = "KAFKA_TLS_ENABLED"          //nolint:staticcheck
	KAFKA_CERT_FILE            = "KAFKA_CERT_FILE"            //nolint:staticcheck
	KAFKA_KEY_FILE             = "KAFKA_KEY_FILE"             //nolint:staticcheck
	KAFKA_CA_FILE              = "KAFKA_CA_FILE"              //nolint:staticcheck
	KAFKA_INSECURE_SKIP_VERIFY = "KAFKA_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	KAFKA_ENABLED = "KAFKA_ENABLED" //nolint:staticcheck
)

// Supported Kafka SASL mechanisms.
const (
	KafkaSASLMechanismPlain       = "plain"
	KafkaSASLMechanismScramSHA256 = "scram-sha-256"
	KafkaSASLMechanismScramSHA512 = "scram-sha-512"
)

// Kafka configures the framework-managed Kafka client backed by franz-go.
//
// Only environment-level settings are exposed: where to connect and how to
// authenticate. Behavioral tuning (acks, compression, batching, balancing,
// offsets, protocol version) deliberately stays with franz-go defaults, which
// already implement current best practices (idempotent producer, all-ISR acks,
// cooperative-sticky balancing, automatic version negotiation). Callers that
// need specific behavior pass extra kgo.Opt values to provider/kafka.New.
type Kafka struct {
	Brokers  []string `json:"brokers" mapstructure:"brokers" ini:"brokers" yaml:"brokers"`
	ClientID string   `json:"client_id" mapstructure:"client_id" ini:"client_id" yaml:"client_id"`

	SASLEnabled   bool   `json:"sasl_enabled" mapstructure:"sasl_enabled" ini:"sasl_enabled" yaml:"sasl_enabled"`
	SASLMechanism string `json:"sasl_mechanism" mapstructure:"sasl_mechanism" ini:"sasl_mechanism" yaml:"sasl_mechanism"`
	SASLUsername  string `json:"sasl_username" mapstructure:"sasl_username" ini:"sasl_username" yaml:"sasl_username"`
	SASLPassword  string `json:"sasl_password" mapstructure:"sasl_password" ini:"sasl_password" yaml:"sasl_password"`

	TLSEnabled         bool   `json:"tls_enabled" mapstructure:"tls_enabled" ini:"tls_enabled" yaml:"tls_enabled"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enabled bool `json:"enabled" mapstructure:"enabled" ini:"enabled" yaml:"enabled"`
}

func (*Kafka) setDefault() {
	cv.SetDefault("kafka.brokers", []string{"127.0.0.1:9092"})
	cv.SetDefault("kafka.client_id", "gst")

	cv.SetDefault("kafka.sasl_enabled", false)
	cv.SetDefault("kafka.sasl_mechanism", KafkaSASLMechanismPlain)
	cv.SetDefault("kafka.sasl_username", "")
	cv.SetDefault("kafka.sasl_password", "")

	cv.SetDefault("kafka.tls_enabled", false)
	cv.SetDefault("kafka.cert_file", "")
	cv.SetDefault("kafka.key_file", "")
	cv.SetDefault("kafka.ca_file", "")
	cv.SetDefault("kafka.insecure_skip_verify", false)

	cv.SetDefault("kafka.enabled", false)
}
