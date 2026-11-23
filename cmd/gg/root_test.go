package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	if err := startCommand(tsCmd, nil); err != nil {
		t.Fatalf("expected project command to run outside the gst framework root: %v", err)
	}
}

// TestCommandErrorsLeaveThePrintingToMain pins that cobra prints nothing for a
// command that fails while it runs, for a command gg does not have, whose
// error carries cobra's suggestions, or for a command line gg cannot run,
// the usage included: run prints the error once, with where the usage is.
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
		if out != "" {
			t.Fatalf("a command line gg cannot run printed %q through cobra, want nothing: main prints the error and where the usage is", out)
		}
	})
}

// TestVersionReportsTheBuild pins what gg --version prints: the module
// version Go recorded when gg was built — the tag it was installed at, a
// pseudo-version for an untagged commit, or "(devel)" for a build that
// recorded none, as this test binary is — never a number written into the
// code.
func TestVersionReportsTheBuild(t *testing.T) {
	info, ok := debug.ReadBuildInfo()
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

// TestRunTellsACommandLineItCannotRunFromACommandThatFailed pins the code gg
// exits with and what it prints last, on standard error: 2 for a command
// line cobra refuses before the command starts — a command, a flag or
// arguments gg does not have, under gg itself, under a group of commands or
// under a command that runs on its own and has commands under it — with
// where the usage is and cobra's suggestion after it; 1 for a command that
// ran and failed, its error alone; 1 and nothing for a failure reported
// already; 0 for a command that went well, the help a group prints when
// called alone included.
func TestRunTellsACommandLineItCannotRunFromACommandThatFailed(t *testing.T) {
	writeGoMod(t, t.TempDir(), "example.com/app")
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	reported := &cobra.Command{Use: "reported", RunE: func(*cobra.Command, []string) error { return errReported }}
	rootCmd.AddCommand(reported)
	t.Cleanup(func() { rootCmd.RemoveCommand(reported) })

	for _, tt := range []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{name: "a command gg does not have", args: []string{"modul"}, wantCode: 2, want: "\n✘ unknown command \"modul\" for \"gg\"\n  Run 'gg --help' for usage.\n\nDid you mean this?\n\tmodule\n"},
		{name: "a command a group does not have", args: []string{"config", "list"}, wantCode: 2, want: "\n✘ unknown command \"list\" for \"gg config\"\n  Run 'gg config --help' for usage.\n"},
		{name: "a command another group does not have", args: []string{"module", "cpy"}, wantCode: 2, want: "\n✘ unknown command \"cpy\" for \"gg module\"\n  Run 'gg module --help' for usage.\n"},
		{name: "a command gg gen does not have", args: []string{"gen", "tss"}, wantCode: 2, want: "\n✘ unknown command \"tss\" for \"gg gen\"\n  Run 'gg gen --help' for usage.\n"},
		{name: "a command gg migrate does not have", args: []string{"migrate", "shcema", "--yes"}, wantCode: 2, want: "\n✘ unknown command \"shcema\" for \"gg migrate\"\n  Run 'gg migrate --help' for usage.\n"},
		{name: "a flag the command does not have", args: []string{"gen", "--nope"}, wantCode: 2, want: "\n✘ unknown flag: --nope\n  Run 'gg gen --help' for usage.\n"},
		{name: "arguments the command does not take", args: []string{"module", "add"}, wantCode: 2, want: "\n✘ accepts 1 arg(s), received 0\n  Run 'gg module add --help' for usage.\n"},
		{name: "a command that fails while it runs", args: []string{"module", "add", "../sample"}, wantCode: 1, want: "\n✘ module command accepts a module name, not a path: ../sample\n"},
		{name: "a failure reported already", args: []string{"reported"}, wantCode: 1},
		{name: "a command that goes well", args: []string{"help", "gen"}},
		{name: "a group called alone", args: []string{"module"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			var stderr string
			captureStdout(t, func() {
				stderr = captureStderr(t, func() { code = run(tt.args) })
			})
			if code != tt.wantCode || stderr != tt.want {
				t.Fatalf("gg %s exited with %d and printed %q, want %d and %q", strings.Join(tt.args, " "), code, stderr, tt.wantCode, tt.want)
			}
		})
	}
}

// TestFrameworkVersionMismatch pins the example frameworkVersionMismatch
// gives, a gg built at one version of the framework in a project requiring
// an older one, which gg upgrade moves; that a gg older than the project is
// told the one way out it has; and where nothing is compared: the two the
// same, a gg built from a working copy, a go.mod replacing the framework or
// requiring none, a Go workspace, and a directory that is no project.
func TestFrameworkVersionMismatch(t *testing.T) {
	const requires = "module example.com/app\n\ngo 1.26\n\nrequire github.com/hydroan/gst v1.2.0\n"
	for _, tt := range []struct {
		name   string
		built  string
		goMod  string
		goWork string
		want   string
	}{
		{name: "a newer gg", built: "v1.3.0", goMod: requires, want: "gg is built from github.com/hydroan/gst v1.3.0, and go.mod requires v1.2.0: what one version generates need not build with the other\n" +
			"  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.2.0\n" +
			"  To move the project to this gg: gg upgrade"},
		{name: "an older gg", built: "v1.1.0", goMod: requires, want: "gg is built from github.com/hydroan/gst v1.1.0, and go.mod requires v1.2.0: what one version generates need not build with the other\n" +
			"  To install the gg the project requires: go install github.com/hydroan/gst/cmd/gg@v1.2.0"},
		{name: "the same version", built: "v1.2.0", goMod: requires},
		{name: "a build recording no version", built: "(devel)", goMod: requires},
		{name: "a build of a working copy with changes", built: "v1.3.0+dirty", goMod: requires},
		{name: "a go.mod replacing the framework", built: "v1.3.0", goMod: requires + "\nreplace github.com/hydroan/gst => ../gst\n"},
		{name: "a go.mod requiring no framework", built: "v1.3.0", goMod: "module example.com/app\n\ngo 1.26\n"},
		{name: "a Go workspace", built: "v1.3.0", goMod: requires, goWork: "go 1.26\n\nuse .\n"},
		{name: "no go.mod", built: "v1.3.0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for name, content := range map[string]string{"go.mod": tt.goMod, "go.work": tt.goWork} {
				if content == "" {
					continue
				}
				if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			err := frameworkVersionMismatch(tt.built)
			switch {
			case tt.want == "" && err != nil:
				t.Fatalf("frameworkVersionMismatch(%s) = %v, want nil", tt.built, err)
			case tt.want != "" && (err == nil || err.Error() != tt.want):
				t.Fatalf("frameworkVersionMismatch(%s) = %v, want %q", tt.built, err, tt.want)
			}
		})
	}
}

// TestStartCommandHoldsProjectCommandsToTheFrameworkVersion pins which
// commands the version of the framework is checked for: a project command
// is refused in a project requiring another version than gg's, while gg new,
// which starts a project, gg upgrade, which moves the project to the version
// of gg, and the commands describing gg, help and completion, run all the
// same.
func TestStartCommandHoldsProjectCommandsToTheFrameworkVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app\n\ngo 1.26\n\nrequire github.com/hydroan/gst v1.2.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	built := rootCmd.Version
	rootCmd.Version = "v1.3.0"
	t.Cleanup(func() { rootCmd.Version = built })

	if _, err := executeRootCommand(t, "module", "list"); err == nil || !strings.HasPrefix(err.Error(), "gg is built from github.com/hydroan/gst v1.3.0, and go.mod requires v1.2.0") {
		t.Fatalf("gg module list error = %v, want the version mismatch", err)
	}
	for _, cmd := range []*cobra.Command{newCmd, upgradeCmd} {
		if err := startCommand(cmd, nil); err != nil {
			t.Fatalf("gg %s is held to no version, got %v", cmd.Name(), err)
		}
	}
	for _, args := range [][]string{{"help", "gen"}, {"completion", "bash"}} {
		if _, err := executeRootCommand(t, args...); err != nil {
			t.Fatalf("gg %s error = %v, want it to run whatever the version", strings.Join(args, " "), err)
		}
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
// --help and --version of the command that ran get their defaults back
// afterwards: a command keeps the flags it parsed for the rest of the
// process, which one test would leak into the next — an earlier --help
// turns every later run into help.
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
		for _, name := range []string{"help", "version"} {
			if flag := cmd.Flags().Lookup(name); flag != nil {
				_ = flag.Value.Set("false")
				flag.Changed = false
			}
		}
	}
	return out.String(), err
}
