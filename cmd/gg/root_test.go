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

	// The guard is asked directly: executing the command would also run the
	// generation it stands for, against a project that has nothing to generate.
	t.Cleanup(func() { tsCmd.SilenceUsage = false })
	if err := startCommand(tsCmd, nil); err != nil {
		t.Fatalf("expected project command to run outside the gst framework root: %v", err)
	}
}

// TestCommandErrorsLeaveThePrintingToMain pins that cobra prints nothing for a
// command that fails while it runs, since main prints the error once, and
// only the usage for a command line gg cannot run, which the usage explains.
func TestCommandErrorsLeaveThePrintingToMain(t *testing.T) {
	writeGoMod(t, t.TempDir(), "example.com/app")

	t.Run("error while the command runs", func(t *testing.T) {
		out, err := executeRootCommand(t, "module", "add", "../sample")
		if err == nil || !strings.Contains(err.Error(), "accepts a module name, not a path") {
			t.Fatalf("gg module add ../sample error = %v, want the path rejected", err)
		}
		if out != "" {
			t.Fatalf("a command failing while it runs printed %q through cobra, want nothing: main prints the error", out)
		}
	})

	t.Run("command line gg cannot run", func(t *testing.T) {
		out, err := executeRootCommand(t, "module", "add")
		if err == nil {
			t.Fatal("gg module add without a name succeeded, want the missing argument reported")
		}
		if !strings.Contains(out, "Usage:") || strings.Contains(out, err.Error()) {
			t.Fatalf("a command line gg cannot run printed %q through cobra, want the usage alone: main prints the error", out)
		}
	})
}

func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// executeRootCommand runs gg with args and returns what cobra printed. The
// command that ran gets its usage back afterwards: running a command silences
// its usage for the rest of the process, which one test would leak into the
// next.
func executeRootCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(args)

	cmd, err := rootCmd.ExecuteC()

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
	if cmd != nil {
		cmd.SilenceUsage = false
	}
	return out.String(), err
}
