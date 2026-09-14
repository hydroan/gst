package instance

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestIDIsTheHostnameAndARandomSuffix proves the identity's shape: the
// hostname the system reports, a dash, and eight hex characters.
func TestIDIsTheHostnameAndARandomSuffix(t *testing.T) {
	hostname, err := os.Hostname()
	require.NoError(t, err)
	hostname = strings.TrimSpace(hostname)
	require.Equal(t, hostname, Hostname())

	suffix, found := strings.CutPrefix(ID(), hostname+"-")
	require.True(t, found, "the identity must start with the hostname: %q", ID())
	require.Len(t, suffix, 2*suffixBytes)
	_, err = hex.DecodeString(suffix)
	require.NoError(t, err, "the suffix must be hex: %q", suffix)
}

// TestIDIsTheSameForTheLifeOfTheProcess proves every reader gets the one
// identity, so the lease holder, the log entries and the tracing resource
// all name the same process.
func TestIDIsTheSameForTheLifeOfTheProcess(t *testing.T) {
	first := ID()
	second := ID()
	require.Equal(t, first, second)
}

// TestEveryStartDrawsItsOwnSuffix proves two starts on one host — two
// processes, or a restart — do not pass for each other.
func TestEveryStartDrawsItsOwnSuffix(t *testing.T) {
	first := identityOf("node-1")
	second := identityOf("node-1")
	require.NotEqual(t, first.id, second.id)
}

// TestUnreportableHostnameKeepsTheShape proves a hostname the system cannot
// report does not break the identity: a placeholder takes its place, the
// suffix still tells processes apart, and Hostname stays empty rather than
// passing the placeholder off as real.
func TestUnreportableHostnameKeepsTheShape(t *testing.T) {
	id := identityOf("")
	require.Empty(t, id.hostname)
	require.True(t, strings.HasPrefix(id.id, unknownHostname+"-"), id.id)
	require.Len(t, id.id, len(unknownHostname)+1+2*suffixBytes)
}
