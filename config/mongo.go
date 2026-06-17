package config

import "time"

type ReadConcern string

const (
	ReadConcernLocal        ReadConcern = "local"
	ReadConcernMajority     ReadConcern = "majority"
	ReadConcernAvailable    ReadConcern = "available"
	ReadConcernLinearizable ReadConcern = "linearizable"
	ReadConcernSnapshot     ReadConcern = "snapshot"
)

type WriteConcern string

const (
	WriteConcernMajority  WriteConcern = "majority"
	WriteConcernJournaled WriteConcern = "journaled"
	WriteConcernW0        WriteConcern = "0"
	WriteConcernW1        WriteConcern = "1"
	WriteConcernW2        WriteConcern = "2"
	WriteConcernW3        WriteConcern = "3"
	WriteConcernW4        WriteConcern = "4"
	WriteConcernW5        WriteConcern = "5"
	WriteConcernW6        WriteConcern = "6"
	WriteConcernW7        WriteConcern = "7"
	WriteConcernW8        WriteConcern = "8"
	WriteConcernW9        WriteConcern = "9"
)

const (
	MONGO_HOST          = "MONGO_HOST"          //nolint:staticcheck
	MONGO_PORT          = "MONGO_PORT"          //nolint:staticcheck
	MONGO_USERNAME      = "MONGO_USERNAME"      //nolint:staticcheck
	MONGO_PASSWORD      = "MONGO_PASSWORD"      //nolint:staticcheck
	MONGO_DATABASE      = "MONGO_DATABASE"      //nolint:staticcheck
	MONGO_AUTH_SOURCE   = "MONGO_AUTH_SOURCE"   //nolint:staticcheck
	MONGO_MAX_POOL_SIZE = "MONGO_MAX_POOL_SIZE" //nolint:staticcheck
	MONGO_MIN_POOL_SIZE = "MONGO_MIN_POOL_SIZE" //nolint:staticcheck

	MONGO_CONNECT_TIMEOUT          = "MONGO_CONNECT_TIMEOUT"          //nolint:staticcheck
	MONGO_SERVER_SELECTION_TIMEOUT = "MONGO_SERVER_SELECTION_TIMEOUT" //nolint:staticcheck
	MONGO_MAX_CONN_IDLE_TIME       = "MONGO_MAX_CONN_IDLE_TIME"       //nolint:staticcheck
	MONGO_MAX_CONNECTING           = "MONGO_MAX_CONNECTING"           //nolint:staticcheck

	MONGO_READ_CONCERN  = "MONGO_READ_CONCERN"  //nolint:staticcheck
	MONGO_WRITE_CONCERN = "MONGO_WRITE_CONCERN" //nolint:staticcheck

	MONGO_ENABLE_TLS           = "MONGO_ENABLE_TLS"           //nolint:staticcheck
	MONGO_CERT_FILE            = "MONGO_CERT_FILE"            //nolint:staticcheck
	MONGO_KEY_FILE             = "MONGO_KEY_FILE"             //nolint:staticcheck
	MONGO_CA_FILE              = "MONGO_CA_FILE"              //nolint:staticcheck
	MONGO_INSECURE_SKIP_VERIFY = "MONGO_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	MONGO_ENABLE = "MONGO_ENABLE" //nolint:staticcheck
)

type Mongo struct {
	Host        string `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port        int    `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	Username    string `json:"username" mapstructure:"username" ini:"username" yaml:"username"`
	Password    string `json:"password" mapstructure:"password" ini:"password" yaml:"password"`
	Database    string `json:"database" mapstructure:"database" ini:"database" yaml:"database"`
	AuthSource  string `json:"auth_source" mapstructure:"auth_source" ini:"auth_source" yaml:"auth_source"`
	MaxPoolSize uint64 `json:"max_pool_size" mapstructure:"max_pool_size" ini:"max_pool_size" yaml:"max_pool_size"`
	MinPoolSize uint64 `json:"min_pool_size" mapstructure:"min_pool_size" ini:"min_pool_size" yaml:"min_pool_size"`

	ConnectTimeout         time.Duration `json:"connect_timeout" mapstructure:"connect_timeout" ini:"connect_timeout" yaml:"connect_timeout"`
	ServerSelectionTimeout time.Duration `json:"server_selection_timeout" mapstructure:"server_selection_timeout" ini:"server_selection_timeout" yaml:"server_selection_timeout"`
	MaxConnIdleTime        time.Duration `json:"max_conn_idle_time" mapstructure:"max_conn_idle_time" ini:"max_conn_idle_time" yaml:"max_conn_idle_time"`
	MaxConnecting          uint64        `json:"max_connecting" mapstructure:"max_connecting" ini:"max_connecting" yaml:"max_connecting"`

	ReadConcern  ReadConcern  `json:"read_concern" mapstructure:"read_concern" ini:"read_concern" yaml:"read_concern"`
	WriteConcern WriteConcern `json:"write_concern" mapstructure:"write_concern" ini:"write_concern" yaml:"write_concern"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

func (*Mongo) setDefault() {
	cv.SetDefault("mongo.host", "127.0.0.1")
	cv.SetDefault("mongo.port", 27017)
	cv.SetDefault("mongo.username", "")
	cv.SetDefault("mongo.password", "")
	cv.SetDefault("mongo.database", "")
	cv.SetDefault("mongo.auth_source", "admin")
	cv.SetDefault("mongo.max_pool_size", 0)
	cv.SetDefault("mongo.min_pool_size", 0)

	cv.SetDefault("mongo.connect_timeout", 0)
	cv.SetDefault("mongo.server_selection_timeout", 0)
	cv.SetDefault("mongo.max_conn_idle_time", 0)
	cv.SetDefault("mongo.max_connecting", 0)

	cv.SetDefault("mongo.read_concern", "")
	cv.SetDefault("mongo.write_concern", "")

	cv.SetDefault("mongo.enable_tls", false)
	cv.SetDefault("mongo.cert_file", "")
	cv.SetDefault("mongo.key_file", "")
	cv.SetDefault("mongo.ca_file", "")
	cv.SetDefault("mongo.insecure_skip_verify", false)

	cv.SetDefault("mongo.enable", false)
}
