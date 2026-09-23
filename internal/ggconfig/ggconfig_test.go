package ggconfig_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggconfig"
)

func TestLoad(t *testing.T) {
	t.Run("missing file yields empty config", func(t *testing.T) {
		cfg, err := ggconfig.Load(t.TempDir())
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.Version != 1 {
			t.Errorf("Load().Version = %d, want 1", cfg.Version)
		}
		if len(cfg.Gen.Routes.Ignore) != 0 {
			t.Errorf("Load().Gen.Routes.Ignore = %v, want empty", cfg.Gen.Routes.Ignore)
		}
	})

	t.Run("unknown field is rejected", func(t *testing.T) {
		dir := writeConfig(t, `version: 1
gen:
  routes:
    ignroe:
      /api/signup: [POST]
`)
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for unknown field, got nil")
		}
	})

	t.Run("unsupported version is rejected", func(t *testing.T) {
		dir := writeConfig(t, "version: 2\n")
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for version 2, got nil")
		}
	})

	t.Run("missing version is rejected", func(t *testing.T) {
		dir := writeConfig(t, "gen:\n  routes:\n    ignore: {}\n")
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for missing version, got nil")
		}
	})

	t.Run("prune ignore entries are cleaned", func(t *testing.T) {
		dir := writeConfig(t, "version: 1\nprune:\n  ignore:\n    - \" service/iam/ \"\n    - service/record/list.go\n")
		cfg, err := ggconfig.Load(dir)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		want := []string{"service/iam", "service/record/list.go"}
		if !slices.Equal(cfg.Prune.Ignore, want) {
			t.Fatalf("Load().Prune.Ignore = %q, want %q", cfg.Prune.Ignore, want)
		}
	})

	t.Run("invalid prune ignore entries are rejected", func(t *testing.T) {
		for name, entries := range map[string]string{
			"outside service":  "    - model/iam\n",
			"not clean":        "    - service//iam\n",
			"escaping":         "    - ../service/iam\n",
			"empty":            "    - \"\"\n",
			"listed twice":     "    - service/iam\n    - service/iam/\n",
			"the service root": "    - services\n",
		} {
			t.Run(name, func(t *testing.T) {
				dir := writeConfig(t, "version: 1\nprune:\n  ignore:\n"+entries)
				if _, err := ggconfig.Load(dir); err == nil {
					t.Fatal("Load() expected error, got nil")
				}
			})
		}
	})

	t.Run("unknown prune field is rejected", func(t *testing.T) {
		dir := writeConfig(t, "version: 1\nprune:\n  orphan_ignore:\n    - service/iam\n")
		if _, err := ggconfig.Load(dir); err == nil {
			t.Fatal("Load() expected error for unknown field, got nil")
		}
	})
}

func TestPruneConfigIgnores(t *testing.T) {
	cfg := ggconfig.PruneConfig{Ignore: []string{"service/iam", "service/record/list.go"}}
	tests := []struct {
		path string
		want bool
	}{
		{path: "service/iam", want: true},
		{path: filepath.Join("service", "iam", "user", "list.go"), want: true},
		{path: filepath.Join("service", "iamx", "list.go")},
		{path: filepath.Join("service", "record", "list.go"), want: true},
		{path: filepath.Join("service", "record", "get.go")},
		{path: "service"},
	}
	for _, tt := range tests {
		if got := cfg.Ignores(tt.path); got != tt.want {
			t.Errorf("Ignores(%q) = %t, want %t", tt.path, got, tt.want)
		}
	}
}

func TestUnreadFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{ggconfig.FileName, "gst.yml", ".gg.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("version: 1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A directory of the same name is not a file anyone writes settings into.
	if err := os.Mkdir(filepath.Join(dir, ".gst.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	want := []string{".gg.yaml", "gst.yml"}
	if got := ggconfig.UnreadFiles(dir); !slices.Equal(got, want) {
		t.Fatalf("UnreadFiles() = %q, want %q", got, want)
	}
	if got := ggconfig.UnreadFiles(t.TempDir()); len(got) != 0 {
		t.Fatalf("UnreadFiles() = %q for an empty directory, want none", got)
	}
}

func TestIsLegacyPruneSettings(t *testing.T) {
	for name, want := range map[string]bool{".gg.yaml": true, ".gg.yml": true, ".gst.yaml": false, "gst.yml": false} {
		if got := ggconfig.IsLegacyPruneSettings(name); got != want {
			t.Errorf("IsLegacyPruneSettings(%q) = %t, want %t", name, got, want)
		}
	}
}

// writeConfig writes content as the gst.yaml of a new temporary directory and
// returns the directory.
func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ggconfig.FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}
