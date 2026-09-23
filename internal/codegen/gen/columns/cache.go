package columns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"github.com/hydroan/gst/internal/gghelper"
)

// columnsCacheKey hashes everything that can change the resolved columns:
//
//   - the gg binary itself, which changes whenever the framework changes how
//     columns are resolved (a local framework checkout does not show up in
//     go.mod, but reinstalling gg is what makes such a change take effect);
//   - the inspection program, including the module it targets;
//   - the module requirements, which pin the framework and gorm versions;
//   - the content of every model source file, since the models are what carry
//     the columns: the Go files under the model directory but for tests and
//     generated files, walked by the rules a walk over the project's code
//     follows (the Git ignore rules and gghelper.ExcludedDir).
//
// File paths are deliberately excluded: renaming or moving a model file does
// not change a single column, and which file a model belongs to is resolved
// from the project scan rather than from this program, so hashing paths would
// rebuild for a rename that cannot affect the result. Moving a model to
// another package does change the result, but it also rewrites the package
// clause inside the file, which content hashing catches.
//
// A type a model refers to from outside the model directory is not covered.
// Model files are the project's declared home for those types; if a stale
// result is ever suspected, deleting the cache directory forces a fresh
// inspection.
func columnsCacheKey(program string, modelDir string, ignore gghelper.ProjectIgnore) (string, error) {
	digest := sha256.New()
	digest.Write([]byte(program))

	if executable, err := os.Executable(); err == nil {
		if info, statErr := os.Stat(executable); statErr == nil {
			fmt.Fprintf(digest, "gg:%d:%d\n", info.Size(), info.ModTime().UnixNano())
		}
	}

	goMod, err := os.ReadFile("go.mod")
	if err != nil {
		return "", errors.Wrap(err, "read go.mod")
	}
	digest.Write(goMod)

	fileDigests := make([]string, 0)
	err = ignore.Walk(modelDir, func(path string, info os.FileInfo) error {
		if info.IsDir() {
			if gghelper.ExcludedDir(modelDir, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ggconst.ExtensionGo) ||
			strings.HasSuffix(path, ggconst.PatternTestFile) ||
			strings.HasSuffix(path, ggconst.SuffixGenGo) {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fileDigest := sha256.Sum256(content)
		fileDigests = append(fileDigests, hex.EncodeToString(fileDigest[:]))
		return nil
	})
	if err != nil {
		return "", errors.Wrapf(err, "hash model sources under %s", modelDir)
	}
	// The per-file digests are sorted before being folded in, so the key
	// depends on the set of model sources, not on the order the walk visits
	// them nor on what the files are called.
	sort.Strings(fileDigests)
	for _, fileDigest := range fileDigests {
		fmt.Fprintln(digest, fileDigest)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// readColumnsCache returns a previously stored inspection result for the key.
func readColumnsCache(key string) ([]modelColumns, bool) {
	dir, err := columnsCacheDir()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(filepath.Join(dir, key+".json"))
	if err != nil {
		return nil, false
	}
	var resolved []modelColumns
	if err = json.Unmarshal(content, &resolved); err != nil {
		return nil, false
	}
	return resolved, true
}

// writeColumnsCache stores an inspection result, replacing the project's
// previous entry: only the current inputs are ever worth keeping.
func writeColumnsCache(key string, resolved []modelColumns) error {
	dir, err := columnsCacheDir()
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0o750); err != nil {
		return errors.Wrapf(err, "create cache directory %s", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return errors.Wrapf(err, "read cache directory %s", dir)
	}
	for _, entry := range entries {
		if entry.Name() == key+".json" {
			continue
		}
		// #nosec G122 -- the entry comes from reading the cache directory gg owns.
		if err = os.Remove(filepath.Join(dir, entry.Name())); err != nil {
			return errors.Wrapf(err, "remove stale cache entry %s", entry.Name())
		}
	}
	content, err := json.Marshal(resolved)
	if err != nil {
		return errors.Wrap(err, "encode resolved columns")
	}
	if err = os.WriteFile(filepath.Join(dir, key+".json"), content, 0o600); err != nil {
		return errors.Wrap(err, "write column cache")
	}
	return nil
}

// columnsCacheDir returns the per-project cache directory. Results live under
// the user cache directory rather than in the project, so a generated tree
// stays free of tooling state.
func columnsCacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", errors.Wrap(err, "resolve user cache directory")
	}
	project, err := os.Getwd()
	if err != nil {
		return "", errors.Wrap(err, "resolve working directory")
	}
	projectDigest := sha256.Sum256([]byte(project))
	return filepath.Join(base, "gg", "columns", hex.EncodeToString(projectDigest[:8])), nil
}
