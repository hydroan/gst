package config

import (
	"time"
)

const (
	LDAP_HOST            = "LDAP_HOST"            //nolint:staticcheck
	LDAP_PORT            = "LDAP_PORT"            //nolint:staticcheck
	LDAP_BASE_DN         = "LDAP_BASE_DN"         //nolint:staticcheck
	LDAP_BIND_DN         = "LDAP_BIND_DN"         //nolint:staticcheck
	LDAP_BIND_PASSWORD   = "LDAP_BIND_PASSWORD"   //nolint:staticcheck,gosec
	LDAP_ATTRIBUTES      = "LDAP_ATTRIBUTES"      //nolint:staticcheck
	LDAP_FILTER          = "LDAP_FILTER"          //nolint:staticcheck
	LDAP_GROUP_FILTER    = "LDAP_GROUP_FILTER"    //nolint:staticcheck
	LDAP_USER_FILTER     = "LDAP_USER_FILTER"     //nolint:staticcheck
	LDAP_GROUP_DN        = "LDAP_GROUP_DN"        //nolint:staticcheck
	LDAP_USER_DN         = "LDAP_USER_DN"         //nolint:staticcheck
	LDAP_GROUP_ATTRIBUTE = "LDAP_GROUP_ATTRIBUTE" //nolint:staticcheck
	LDAP_USER_ATTRIBUTE  = "LDAP_USER_ATTRIBUTE"  //nolint:staticcheck
	LDAP_SCOPE           = "LDAP_SCOPE"           //nolint:staticcheck
	LDAP_REQUEST_TIMEOUT = "LDAP_REQUEST_TIMEOUT" //nolint:staticcheck
	LDAP_CONN_TIMEOUT    = "LDAP_CONN_TIMEOUT"    //nolint:staticcheck
	LDAP_REFERRALS       = "LDAP_REFERRALS"       //nolint:staticcheck
	LDAP_DEREF           = "LDAP_DEREF"           //nolint:staticcheck
	LDAP_PAGE_SIZE       = "LDAP_PAGE_SIZE"       //nolint:staticcheck
	LDAP_HEARTBEAT       = "LDAP_HEARTBEAT"       //nolint:staticcheck

	LDAP_ENABLE_TLS           = "LDAP_ENABLE_TLS"           //nolint:staticcheck
	LDAP_CERT_FILE            = "LDAP_CERT_FILE"            //nolint:staticcheck
	LDAP_KEY_FILE             = "LDAP_KEY_FILE"             //nolint:staticcheck
	LDAP_CA_FILE              = "LDAP_CA_FILE"              //nolint:staticcheck
	LDAP_INSECURE_SKIP_VERIFY = "LDAP_INSECURE_SKIP_VERIFY" //nolint:staticcheck

	LDAP_ENABLE = "LDAP_ENABLE" //nolint:staticcheck
)

// Scope represents the search scope
type Scope int

const (
	// ScopeBaseObject indicates a base object search
	ScopeBaseObject Scope = 0
	// ScopeSingleLevel indicates a single level search
	ScopeSingleLevel Scope = 1
	// ScopeWholeSubtree indicates a whole subtree search
	ScopeWholeSubtree Scope = 2
)

// DerefAliases controls alias dereferencing
type DerefAliases int

const (
	// NeverDerefAliases never dereferences aliases
	NeverDerefAliases DerefAliases = 0
	// DerefInSearching dereferences aliases in searching
	DerefInSearching DerefAliases = 1
	// DerefFindingBaseObj dereferences aliases in finding the base object
	DerefFindingBaseObj DerefAliases = 2
	// DerefAlways dereferences aliases always
	DerefAlways DerefAliases = 3
)

type Ldap struct {
	Host           string        `json:"host" mapstructure:"host" ini:"host" yaml:"host"`
	Port           int           `json:"port" mapstructure:"port" ini:"port" yaml:"port"`
	BaseDN         string        `json:"base_dn" mapstructure:"base_dn" ini:"base_dn" yaml:"base_dn"`
	BindDN         string        `json:"bind_dn" mapstructure:"bind_dn" ini:"bind_dn" yaml:"bind_dn"`
	BindPassword   string        `json:"bind_password" mapstructure:"bind_password" ini:"bind_password" yaml:"bind_password"`
	Attributes     []string      `json:"attributes" mapstructure:"attributes" ini:"attributes" yaml:"attributes"`
	Filter         string        `json:"filter" mapstructure:"filter" ini:"filter" yaml:"filter"`
	GroupFilter    string        `json:"group_filter" mapstructure:"group_filter" ini:"group_filter" yaml:"group_filter"`
	UserFilter     string        `json:"user_filter" mapstructure:"user_filter" ini:"user_filter" yaml:"user_filter"`
	GroupDN        string        `json:"group_dn" mapstructure:"group_dn" ini:"group_dn" yaml:"group_dn"`
	UserDN         string        `json:"user_dn" mapstructure:"user_dn" ini:"user_dn" yaml:"user_dn"`
	GroupAttribute string        `json:"group_attribute" mapstructure:"group_attribute" ini:"group_attribute" yaml:"group_attribute"`
	UserAttribute  string        `json:"user_attribute" mapstructure:"user_attribute" ini:"user_attribute" yaml:"user_attribute"`
	Scope          int           `json:"scope" mapstructure:"scope" ini:"scope" yaml:"scope"`
	RequestTimeout time.Duration `json:"request_timeout" mapstructure:"request_timeout" ini:"request_timeout" yaml:"request_timeout"`
	ConnTimeout    time.Duration `json:"conn_timeout" mapstructure:"conn_timeout" ini:"conn_timeout" yaml:"conn_timeout"`
	Referrals      bool          `json:"referrals" mapstructure:"referrals" ini:"referrals" yaml:"referrals"`
	Deref          int           `json:"deref" mapstructure:"deref" ini:"deref" yaml:"deref"`
	PageSize       int           `json:"page_size" mapstructure:"page_size" ini:"page_size" yaml:"page_size"`
	Heartbeat      time.Duration `json:"heartbeat" mapstructure:"heartbeat" ini:"heartbeat" yaml:"heartbeat"`

	EnableTLS          bool   `json:"enable_tls" mapstructure:"enable_tls" ini:"enable_tls" yaml:"enable_tls"`
	CertFile           string `json:"cert_file" mapstructure:"cert_file" ini:"cert_file" yaml:"cert_file"`
	KeyFile            string `json:"key_file" mapstructure:"key_file" ini:"key_file" yaml:"key_file"`
	CAFile             string `json:"ca_file" mapstructure:"ca_file" ini:"ca_file" yaml:"ca_file"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify" mapstructure:"insecure_skip_verify" ini:"insecure_skip_verify" yaml:"insecure_skip_verify"`

	Enable bool `json:"enable" mapstructure:"enable" ini:"enable" yaml:"enable"`
}

// setDefault sets default values for the LDAP configuration
func (*Ldap) setDefault() {
	cv.SetDefault("ldap.host", "localhost")
	cv.SetDefault("ldap.port", 389)
	cv.SetDefault("ldap.base_dn", "")
	cv.SetDefault("ldap.bind_dn", "")
	cv.SetDefault("ldap.bind_password", "")
	cv.SetDefault("ldap.attributes", []string{"*"})
	cv.SetDefault("ldap.filter", "(objectClass=*)")
	cv.SetDefault("ldap.group_filter", "(objectClass=groupOfNames)")
	cv.SetDefault("ldap.user_filter", "(objectClass=person)")
	cv.SetDefault("ldap.group_dn", "")
	cv.SetDefault("ldap.user_dn", "")
	cv.SetDefault("ldap.group_attribute", "member")
	cv.SetDefault("ldap.user_attribute", "uid")
	cv.SetDefault("ldap.scope", int(ScopeWholeSubtree))
	cv.SetDefault("ldap.request_timeout", 10*time.Second)
	cv.SetDefault("ldap.conn_timeout", 10*time.Second)
	cv.SetDefault("ldap.referrals", false)
	cv.SetDefault("ldap.deref", int(NeverDerefAliases))
	cv.SetDefault("ldap.page_size", 1000)
	cv.SetDefault("ldap.heartbeat", 30*time.Second)

	cv.SetDefault("ldap.enable_tls", false)
	cv.SetDefault("ldap.cert_file", "")
	cv.SetDefault("ldap.key_file", "")
	cv.SetDefault("ldap.ca_file", "")
	cv.SetDefault("ldap.insecure_skip_verify", false)

	cv.SetDefault("ldap.enable", false)
}
