package nats

import (
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

func TestConnectOptionsMaxReconnectsVerbatim(t *testing.T) {
	// The configured value must reach the client untranslated: -1 keeps
	// reconnecting forever, 0 disables reconnecting, a positive value bounds
	// the attempts. A > 0 gate would silently swap -1 and 0 for the NATS
	// library default of 60 bounded attempts, which permanently closes the
	// connection once spent.
	for _, configured := range []int{-1, 0, 7} {
		opts, err := connectOptions(config.Nats{MaxReconnects: configured})
		require.NoError(t, err)

		applied := nats.GetDefaultOptions()
		for _, opt := range opts {
			require.NoError(t, opt(&applied))
		}
		require.Equal(t, configured, applied.MaxReconnect)
	}
}
