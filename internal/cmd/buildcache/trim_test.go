package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestTrimCacheRemovesWhatWentUnusedLongerThanTheAge pins what a trim
// touches: index files, data files and executable directories of the
// cache's entries whose modification time is older than the age, and
// nothing else in the cache; and that a trim within the interval of the
// last one is skipped.
func TestTrimCacheRemovesWhatWentUnusedLongerThanTheAge(t *testing.T) {
	cacheDir := t.TempDir()
	now := time.Now()
	old, fresh := now.Add(-72*time.Hour), now.Add(-time.Hour)
	entry := func(prefix string, suffix string) string {
		return filepath.Join(cacheDir, prefix, prefix+strings.Repeat("0", 62)+"-"+suffix)
	}
	writeCacheFile(t, entry("ab", "a"), "index", old)
	writeCacheFile(t, entry("cd", "d"), "data of six", old)
	executable := entry("ef", "d")
	writeCacheFile(t, filepath.Join(executable, "main"), "a program", old)
	require.NoError(t, os.Chtimes(executable, old, old))
	writeCacheFile(t, entry("12", "a"), "fresh index", fresh)
	writeCacheFile(t, filepath.Join(cacheDir, "ab", "notes.txt"), "kept: not an entry", old)
	writeCacheFile(t, filepath.Join(cacheDir, "README"), "kept: go's own", old)

	removed, freed, err := trimCache(cacheDir, 48*time.Hour, 24*time.Hour, now)
	require.NoError(t, err)
	require.Equal(t, 3, removed)
	require.Equal(t, int64(len("index")+len("data of six")+len("a program")), freed)
	for _, gone := range []string{entry("ab", "a"), entry("cd", "d"), executable} {
		require.NoFileExists(t, gone)
	}
	for _, kept := range []string{entry("12", "a"), filepath.Join(cacheDir, "ab", "notes.txt"), filepath.Join(cacheDir, "README")} {
		require.FileExists(t, kept)
	}
	require.FileExists(t, filepath.Join(cacheDir, trimMarker))

	// Within the interval the trim is skipped, however old the entries.
	writeCacheFile(t, entry("34", "d"), "aged since", old)
	removed, _, err = trimCache(cacheDir, 48*time.Hour, 24*time.Hour, now.Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, removed)
	require.FileExists(t, entry("34", "d"))
}

func writeCacheFile(t *testing.T, path, content string, modTime time.Time) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}
