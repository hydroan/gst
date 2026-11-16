package router

import (
	"path"
	"testing"

	"github.com/hydroan/gst/config"
	"github.com/stretchr/testify/require"
)

// TestInitRefusesAMalformedWebSocketOrigin proves a server.websocket_origins
// pattern the WebSocket library could not read stops the start, by name,
// before anything of the engine is built: served, the pattern would show
// only as the refusal of the first handshake from another origin.
func TestInitRefusesAMalformedWebSocketOrigin(t *testing.T) {
	engine, origins := root, config.App.Server.WebSocketOrigins
	t.Cleanup(func() { config.App.Server.WebSocketOrigins = origins })
	config.App.Server.WebSocketOrigins = []string{"*.sample.example", "[sample.example"}

	err := Init()
	require.ErrorIs(t, err, path.ErrBadPattern)
	require.ErrorContains(t, err, `invalid server.websocket_origins: malformed origin pattern "[sample.example"`)
	require.Same(t, engine, root, "a start refused builds no engine")
}
