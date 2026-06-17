package config

import "github.com/hydroan/gst/types/consts"

const (
	AUDIT_ENABLE             = "AUDIT_ENABLE"             //nolint:staticcheck
	AUDIT_ASYNC_WRITE        = "AUDIT_ASYNC_WRITE"        //nolint:staticcheck
	AUDIT_BATCH_SIZE         = "AUDIT_BATCH_SIZE"         //nolint:staticcheck
	AUDIT_FLUSH_INTERVAL     = "AUDIT_FLUSH_INTERVAL"     //nolint:staticcheck
	AUDIT_EXCLUDE_OPERATIONS = "AUDIT_EXCLUDE_OPERATIONS" //nolint:staticcheck

	AUDIT_EXCLUDE_TABLES       = "AUDIT_EXCLUDE_TABLES"       //nolint:staticcheck
	AUDIT_RECORD_REQUEST_BODY  = "AUDIT_RECORD_REQUEST_BODY"  //nolint:staticcheck
	AUDIT_RECORD_RESPONSE_BODY = "AUDIT_RECORD_RESPONSE_BODY" //nolint:staticcheck
	AUDIT_RECORD_OLD_VALUES    = "AUDIT_RECORD_OLD_VALUES"    //nolint:staticcheck
	AUDIT_RECORD_NEW_VALUES    = "AUDIT_RECORD_NEW_VALUES"    //nolint:staticcheck
	AUDIT_EXCLUDE_FIELDS       = "AUDIT_EXCLUDE_FIELDS"       //nolint:staticcheck
	AUDIT_INCLUDE_FIELDS       = "AUDIT_INCLUDE_FIELDS"       //nolint:staticcheck
	AUDIT_MAX_FIELD_LENGTH     = "AUDIT_MAX_FIELD_LENGTH"     //nolint:staticcheck
	AUDIT_RECORD_QUERY_PARAMS  = "AUDIT_RECORD_QUERY_PARAMS"  //nolint:staticcheck
	AUDIT_RECORD_USER_AGENT    = "AUDIT_RECORD_USER_AGENT"    //nolint:staticcheck
)

type Audit struct {
	Enable            bool        `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
	AsyncWrite        bool        `json:"async_write" mapstructure:"async_write" ini:"async_write" yaml:"async_write"`
	BatchSize         int         `json:"batch_size" mapstructure:"batch_size" ini:"batch_size" yaml:"batch_size"`
	FlushInterval     string      `json:"flush_interval" mapstructure:"flush_interval" ini:"flush_interval" yaml:"flush_interval"`
	ExcludeOperations []consts.OP `json:"exclude_operations" mapstructure:"exclude_operations" ini:"exclude_operations" yaml:"exclude_operations"`

	ExcludeTables      []string `json:"exclude_tables" mapstructure:"exclude_tables" ini:"exclude_tables" yaml:"exclude_tables"`
	RecordRequestBody  bool     `json:"record_request_body" mapstructure:"record_request_body" ini:"record_request_body" yaml:"record_request_body"`
	RecordResponseBody bool     `json:"record_response_body" mapstructure:"record_response_body" ini:"record_response_body" yaml:"record_response_body"`
	RecordOldValues    bool     `json:"record_old_values" mapstructure:"record_old_values" ini:"record_old_values" yaml:"record_old_values"`
	RecordNewValues    bool     `json:"record_new_values" mapstructure:"record_new_values" ini:"record_new_values" yaml:"record_new_values"`
	ExcludeFields      []string `json:"exclude_fields" mapstructure:"exclude_fields" ini:"exclude_fields" yaml:"exclude_fields"`
	IncludeFields      []string `json:"include_fields" mapstructure:"include_fields" ini:"include_fields" yaml:"include_fields"`
	MaxFieldLength     int      `json:"max_field_length" mapstructure:"max_field_length" ini:"max_field_length" yaml:"max_field_length"`
	RecordQueryParams  bool     `json:"record_query_params" mapstructure:"record_query_params" ini:"record_query_params" yaml:"record_query_params"`
	RecordUserAgent    bool     `json:"record_user_agent" mapstructure:"record_user_agent" ini:"record_user_agent" yaml:"record_user_agent"`
}

func (*Audit) setDefault() {
	cv.SetDefault("audit.enable", false)
	cv.SetDefault("audit.async_write", true)
	cv.SetDefault("audit.batch_size", 10000)
	cv.SetDefault("audit.flush_interval", "5s")
	cv.SetDefault("audit.exclude_operations", []consts.OP{consts.OP_LIST, consts.OP_GET})

	cv.SetDefault("audit.exclude_tables", []string{})
	cv.SetDefault("audit.record_request_body", true)
	cv.SetDefault("audit.record_response_body", true)
	cv.SetDefault("audit.record_old_values", true)
	cv.SetDefault("audit.record_new_values", true)
	cv.SetDefault("audit.exclude_fields", []string{"password", "passwd", "pwd", "secret", "token", "key", "private_key"})
	cv.SetDefault("audit.include_fields", []string{})
	cv.SetDefault("audit.max_field_length", 1000)
	cv.SetDefault("audit.record_query_params", true)
	cv.SetDefault("audit.record_user_agent", true)
}
