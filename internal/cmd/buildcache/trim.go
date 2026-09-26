package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
)

// trim removes the entries of the build cache unused for longer than -age,
// at most once every -every: a marker file in the cache records the last
// trim, the way go's own trim.txt does for its five-day trim.
func trim(args []string) error {
	flags := flag.NewFlagSet("trim", flag.ContinueOnError)
	age := flags.Duration("age", 48*time.Hour, "remove entries unused for longer than this")
	every := flags.Duration("every", 24*time.Hour, "trim at most this often")
	if err := flags.Parse(args); err != nil {
		return errors.WithStack(err)
	}
	cacheDir, err := goEnv("GOCACHE")
	if err != nil {
		return err
	}
	removed, freed, err := trimCache(cacheDir, *age, *every, time.Now())
	if err != nil {
		return err
	}
	if removed > 0 {
		log.Printf("removed %d cache entries unused for more than %v, %.1f GB", removed, *age, float64(freed)/1e9)
	}
	return nil
}

// trimMarker records in the cache when the last trim ran.
const trimMarker = "buildcache-trim.txt"

// cacheEntryName matches the files and directories go keeps cache entries
// in, inside the two-hex-digit subdirectories: index files, data files and
// the directories of cached executables.
var cacheEntryName = regexp.MustCompile(`^[0-9a-f]{64}-[ad]$`)

// trimCache removes the cache entries under cacheDir whose modification
// time, which go refreshes on every use, is older than age, unless a trim
// ran less than every ago. It returns how many entries it removed and their
// size.
func trimCache(cacheDir string, age, every time.Duration, now time.Time) (int, int64, error) {
	marker := filepath.Join(cacheDir, trimMarker)
	if info, err := os.Stat(marker); err == nil && now.Sub(info.ModTime()) < every {
		return 0, 0, nil
	}
	cutoff := now.Add(-age)
	removed, freed := 0, int64(0)
	subdirs, err := os.ReadDir(cacheDir)
	if err != nil {
		return 0, 0, errors.WithStack(err)
	}
	for _, subdir := range subdirs {
		if !subdir.IsDir() || len(subdir.Name()) != 2 || strings.Trim(subdir.Name(), "0123456789abcdef") != "" {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(cacheDir, subdir.Name()))
		if err != nil {
			return removed, freed, errors.WithStack(err)
		}
		for _, entry := range entries {
			if !cacheEntryName.MatchString(entry.Name()) {
				continue
			}
			path := filepath.Join(cacheDir, subdir.Name(), entry.Name())
			info, err := os.Stat(path)
			if err != nil || !info.ModTime().Before(cutoff) {
				continue
			}
			size := info.Size()
			if info.IsDir() {
				size = dirSize(path)
			}
			if err := os.RemoveAll(path); err != nil {
				return removed, freed, errors.WithStack(err)
			}
			removed++
			freed += size
		}
	}
	return removed, freed, errors.WithStack(os.WriteFile(marker, []byte(now.Format(time.RFC3339)+"\n"), 0o600))
}

// dirSize sums the files of a cached executable's directory.
func dirSize(dir string) int64 {
	var size int64
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, infoErr := d.Info(); infoErr == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}
