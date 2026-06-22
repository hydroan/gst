package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFrameworkRootGuardBlocksProjectCommands(t *testing.T) {
	writeGoMod(t, t.TempDir(), "github.com/hydroan/gst")

	_, err := executeRootCommand(t, "gen", "ts")
	if err == nil {
		t.Fatal("expected project command to fail in the gst framework root")
	}
	if !strings.Contains(err.Error(), "cannot run in the gst framework repository root") {
		t.Fatalf("expected framework root error, got %v", err)
	}
}

func TestFrameworkRootGuardAllowsMetadataCommands(t *testing.T) {
	writeGoMod(t, t.TempDir(), "github.com/hydroan/gst")

	if _, err := executeRootCommand(t, "--help"); err != nil {
		t.Fatalf("expected help to run in the gst framework root: %v", err)
	}
	if _, err := executeRootCommand(t, "help", "gen"); err != nil {
		t.Fatalf("expected command help to run in the gst framework root: %v", err)
	}
	if _, err := executeRootCommand(t, "--version"); err != nil {
		t.Fatalf("expected version to run in the gst framework root: %v", err)
	}
}

func TestFrameworkRootGuardAllowsProjectCommandsOutsideFrameworkRoot(t *testing.T) {
	writeGoMod(t, t.TempDir(), "example.com/app")

	if _, err := executeRootCommand(t, "gen", "ts"); err != nil {
		t.Fatalf("expected project command to run outside the gst framework root: %v", err)
	}
}

func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

func executeRootCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	err := rootCmd.Execute()

	rootCmd.SetArgs(nil)
	rootCmd.SilenceErrors = false
	rootCmd.SilenceUsage = false
	return out.String(), err
}
