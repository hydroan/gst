//go:build cgo

package database

import (
	"testing"

	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
)

// TestSQLiteTimestampFormatsFollowTheDriver pins the copied format list to
// the driver's own, which only a cgo build compiles: a driver release
// changing the timestamp shapes it writes and accepts fails here rather than
// in a scan.
func TestSQLiteTimestampFormatsFollowTheDriver(t *testing.T) {
	require.Equal(t, sqlite3.SQLiteTimestampFormats, sqliteTimestampFormats)
}
