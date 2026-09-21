package testutil

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestURLTargetsTheTestServerPort(t *testing.T) {
	require.Equal(t, fmt.Sprintf("http://127.0.0.1:%d/api/samples", serverPort), URL("/api/samples"))
}
