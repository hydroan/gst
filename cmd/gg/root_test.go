package main

import (
	"bytes"
	"os"
	"path/filepath"
	rtdebug "runtime/debug"
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
// command that fails while it runs, or for a command gg does not have, whose
// error carries cobra's suggestions: main prints the error once. For a
// command line gg cannot run, cobra prints only the usage, which explains it.
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

	t.Run("command gg does not have", func(t *testing.T) {
		out, err := executeRootCommand(t, "modul")
		if err == nil || !strings.Contains(err.Error(), `unknown command "modul"`) || !strings.Contains(err.Error(), "Did you mean this?\n\tmodule") {
			t.Fatalf("gg modul error = %v, want the unknown command reported with module suggested", err)
		}
		if out != "" {
			t.Fatalf("a command gg does not have printed %q through cobra, want nothing: main prints the error, suggestion included", out)
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

// TestVersionReportsTheBuild pins what gg --version prints: the module
// version Go recorded when gg was built — the tag it was installed at, a
// pseudo-version for an untagged commit, or "(devel)" for a build that
// recorded none, as this test binary is — never a number written into the
// code.
func TestVersionReportsTheBuild(t *testing.T) {
	info, ok := rtdebug.ReadBuildInfo()
	if !ok {
		t.Fatal("the test binary carries no build information")
	}

	out, err := executeRootCommand(t, "--version")
	if err != nil {
		t.Fatalf("gg --version failed: %v", err)
	}
	if want := "gg version " + info.Main.Version + "\n"; out != want {
		t.Fatalf("gg --version printed %q, want %q", out, want)
	}
}

func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module "+modulePath+"\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// executeRootCommand runs gg with args and returns what cobra printed. The
// command that ran gets its usage back afterwards, and its --help and
// --version their defaults: running a command silences its usage and keeps
// the flags it parsed for the rest of the process, which one test would leak
// into the next — an earlier --help turns every later run into help.
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
		for _, name := range []string{"help", "version"} {
			if flag := cmd.Flags().Lookup(name); flag != nil {
				_ = flag.Value.Set("false")
				flag.Changed = false
			}
		}
	}
	return out.String(), err
}
