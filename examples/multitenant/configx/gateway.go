package configx

// GATEWAY_TENANT_HEADER is the environment variable the key of the gateway
// section resolves from.
const GATEWAY_TENANT_HEADER = "GATEWAY_TENANT_HEADER"

// Gateway configures what a trusted gateway in front of the listeners
// supplies, the section [gateway] of config.ini.
type Gateway struct {
	// TenantHeader is the header, and the gRPC metadata key, the gateway
	// names the tenant in; empty leaves the session's tenant alone. It is
	// set only behind a gateway that strips the header from what clients
	// send, since the value is trusted as it stands.
	TenantHeader string `json:"tenant_header" mapstructure:"tenant_header"`
}
