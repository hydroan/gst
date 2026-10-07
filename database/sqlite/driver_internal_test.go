package sqlite

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTimesAreStoredAndReadInUTC pins how a time is stored: whatever offset
// the value carries when it is bound, the text sqlite keeps is its instant in
// UTC, so rows sort by instant like on the other dialects, and the time reads
// back as that instant in UTC (see utcConn).
func TestTimesAreStoredAndReadInUTC(t *testing.T) {
	withPoolLifetime(t, 0)
	db, _ := openMemoryDatabase(t)
	require.NoError(t, db.Exec("CREATE TABLE utc_samples (id INTEGER PRIMARY KEY, at DATETIME)").Error)
	t.Cleanup(func() { require.NoError(t, db.Exec("DROP TABLE IF EXISTS utc_samples").Error) })

	later := time.Date(2026, 1, 1, 20, 0, 0, 0, time.UTC)
	earlier := time.Date(2026, 1, 2, 0, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	require.NoError(t, db.Exec("INSERT INTO utc_samples (id, at) VALUES (1, ?), (2, ?)", later, earlier).Error)

	var texts []string
	require.NoError(t, db.Raw("SELECT CAST(at AS TEXT) FROM utc_samples ORDER BY id").Scan(&texts).Error)
	require.Equal(t, []string{"2026-01-01 20:00:00+00:00", "2026-01-01 16:00:00+00:00"}, texts, "the stored text is the instant in UTC")

	var ordered []int64
	require.NoError(t, db.Raw("SELECT id FROM utc_samples ORDER BY at").Scan(&ordered).Error)
	require.Equal(t, []int64{2, 1}, ordered, "rows sort by instant")

	var read time.Time
	require.NoError(t, db.Raw("SELECT at FROM utc_samples WHERE id = 2").Scan(&read).Error)
	require.True(t, read.Equal(earlier))
	require.Equal(t, time.UTC, read.Location())
}
