package dbruntime

import (
	"net"
	"strconv"

	"github.com/cockroachdb/errors"
)

// ParseReplicaEndpoint splits one configured replica entry into host and
// port. The accepted shape is exactly "host:port" — a replica shares every
// other connection setting with the primary, so the address is all an entry
// may carry. A malformed entry fails database initialization instead of
// being skipped: a replica that silently never joined would disguise a
// configuration typo as "all reads on the primary".
func ParseReplicaEndpoint(endpoint string) (host string, port uint, err error) {
	host, portText, err := net.SplitHostPort(endpoint)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid replica endpoint %q, want host:port", endpoint)
	}
	parsed, err := strconv.ParseUint(portText, 10, 16)
	if err != nil {
		return "", 0, errors.Wrapf(err, "invalid replica endpoint %q, want host:port", endpoint)
	}
	return host, uint(parsed), nil
}
