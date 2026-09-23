// Package ggconfig loads the project-level gst configuration file (gst.yaml)
// that gg commands consume at build time. It is unrelated to the runtime
// configuration managed by the config package.
package ggconfig

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/ggconst"
	"gopkg.in/yaml.v3"
)

// FileName is the name of the project-level gst configuration file,
// located next to go.mod in a business project.
const FileName = "gst.yaml"

// currentVersion is the only gst.yaml schema version supported by this build.
const currentVersion = 1

// Config is the project-level gst configuration.
type Config struct {
	// Version is the gst.yaml schema version. Must be 1.
	Version int `yaml:"version"`

	// Gen configures gg gen behavior.
	Gen GenConfig `yaml:"gen"`

	// Prune configures gg prune, and gg gen --prune, which prunes the same
	// way.
	Prune PruneConfig `yaml:"prune"`
}

// GenConfig configures gg gen behavior.
type GenConfig struct {
	// Routes configures route generation behavior.
	Routes GenRoutesConfig `yaml:"routes"`

	// Models configures model registration generation behavior.
	Models GenModelsConfig `yaml:"models"`
}

// GenRoutesConfig configures route generation behavior.
type GenRoutesConfig struct {
	// Ignore maps route paths to the HTTP methods excluded from code
	// generation. A matched action drops out of the generated registration
	// files while its service file stays on disk.
	Ignore RouteIgnoreRules `yaml:"ignore"`
}

// GenModelsConfig configures model registration generation behavior.
type GenModelsConfig struct {
	// Ignore lists models excluded from the generated model.Register calls.
	// A matched model keeps its routes, services, and generated files; only
	// its registration (and with it table creation) disappears.
	Ignore ModelIgnoreRules `yaml:"ignore"`
}

// PruneConfig configures gg prune.
type PruneConfig struct {
	// Ignore lists the paths gg prune never deletes, whatever the reason it
	// would: a disabled action's service file, a file in an orphan service
	// directory, a directory left empty, or the middleware of a removed copied
	// module. Each entry is a path under service/ or middleware/, relative to
	// the project root, and matches by directory level, the way the from field
	// of an ignore rule does: "service/iam" covers service/iam and everything
	// below it but not service/iamx, and "service/record/list.go" covers that
	// one file. Entries are plain paths: no wildcards, no regular expressions.
	Ignore []string `yaml:"ignore"`
}

// Ignores reports whether path is an Ignore entry or lies below one. With
// the entry "service/iam", "service/iam" and "service/iam/user/list.go" are
// ignored, while "service/iamx/list.go" is not.
func (c PruneConfig) Ignores(path string) bool {
	return slices.ContainsFunc(c.Ignore, func(entry string) bool {
		return underPath(entry, path)
	})
}

// Load reads the gst.yaml file from dir. A missing file is not an error
// and yields a configuration with only defaults, so projects without a
// gst.yaml keep the current gg behavior.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Version: currentVersion}, nil
		}
		return nil, errors.Wrapf(err, "failed to read %s", path)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	cfg := new(Config)
	if err := decoder.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, errors.Wrapf(err, "failed to parse %s", path)
	}
	if cfg.Version != currentVersion {
		return nil, errors.Newf("%s: unsupported version %d, want %d", path, cfg.Version, currentVersion)
	}
	if err := validateRouteIgnoreRules(cfg.Gen.Routes.Ignore); err != nil {
		return nil, errors.Wrapf(err, "%s: gen.routes.ignore", path)
	}
	if err := validatePruneIgnore(&cfg.Prune); err != nil {
		return nil, errors.Wrapf(err, "%s: prune.ignore", path)
	}
	return cfg, nil
}

// validatePruneIgnore cleans every prune.ignore entry in place and rejects an
// entry that is not a clean relative path, lies outside service/ and
// middleware/, the directories gg prune deletes from, or repeats another
// entry.
func validatePruneIgnore(c *PruneConfig) error {
	seen := make(map[string]bool, len(c.Ignore))
	for i, entry := range c.Ignore {
		cleaned, ok := cleanRelativePath(entry)
		if !ok {
			return errors.Newf("entry %q: want a relative path like \"%s/sample\"", entry, ggconst.DirService)
		}
		if !underPath(ggconst.DirService, cleaned) && !underPath(ggconst.DirMiddleware, cleaned) {
			return errors.Newf("entry %q is outside %s/ and %s/, the directories gg prune deletes from", entry, ggconst.DirService, ggconst.DirMiddleware)
		}
		if seen[cleaned] {
			return errors.Newf("entry %q is listed twice", entry)
		}
		seen[cleaned] = true
		c.Ignore[i] = cleaned
	}
	return nil
}

// legacyPruneSettingsFiles held the prune settings of earlier gg releases,
// which the prune section of gst.yaml replaces.
var legacyPruneSettingsFiles = []string{".gg.yaml", ".gg.yml"}

// unreadFileNames are the files gg finds next to gst.yaml but never reads: the
// legacy prune settings files, and names gst.yaml is easily mistaken for.
var unreadFileNames = append(slices.Clone(legacyPruneSettingsFiles), ".gst.yaml", ".gst.yml", "gst.yml")

// UnreadFiles returns the files in dir that look like gg configuration but
// that gg does not read, for the command to warn about: a project holding
// .gg.yaml and gst.yml next to gst.yaml gets [".gg.yaml", "gst.yml"].
func UnreadFiles(dir string) []string {
	var unread []string
	for _, name := range unreadFileNames {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && !info.IsDir() {
			unread = append(unread, name)
		}
	}
	return unread
}

// IsLegacyPruneSettings reports whether name, as UnreadFiles returns it, is
// the prune settings file of earlier gg releases: ".gg.yaml" is, "gst.yml"
// is not.
func IsLegacyPruneSettings(name string) bool {
	return slices.Contains(legacyPruneSettingsFiles, name)
}

// normalizeFromDir cleans and validates a "from" directory prefix of an
// ignore entry. It must be a relative directory such as "model/iam".
func normalizeFromDir(from string) (string, error) {
	cleaned, ok := cleanRelativePath(from)
	switch {
	case cleaned == "":
		return "", errors.New("empty from; drop the field to match all models")
	case !ok:
		return "", errors.Newf("invalid from %q: want a relative directory like \"model/iam\"", cleaned)
	}
	return cleaned, nil
}

// cleanRelativePath trims the spaces and slashes around p and returns what
// remains when it is a clean relative path that stays inside the project:
// " /model/iam/ " gives "model/iam", while "", "model//iam", "./model" and
// "../model" are rejected. A rejected path comes back trimmed, for the
// caller's error message.
func cleanRelativePath(p string) (string, bool) {
	p = strings.Trim(strings.TrimSpace(p), "/")
	cleaned := filepath.ToSlash(filepath.Clean(p))
	if p == "" || cleaned != p || strings.HasPrefix(cleaned, "..") {
		return p, false
	}
	return cleaned, true
}

// underPath reports whether path is prefix or lies below it, comparing whole
// path elements: under "model/iam", "model/iam" and "model/iam/user.go" are,
// while "model/iamx/user.go" is not. An empty prefix holds every path. The
// from field of the ignore rules and the prune.ignore entries match through
// it alike.
func underPath(prefix, path string) bool {
	if prefix == "" {
		return true
	}
	path = filepath.ToSlash(path)
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}
