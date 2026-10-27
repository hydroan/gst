package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWritebackFilesTheResultUnderTheLinkKey pins the whole mechanism on the
// toolchain at hand, for a run without test flags and for one with -short,
// whose result go keys by -test.short=true as well. A change the linker drops
// from a test binary leaves the binary linked again on every cached run,
// since go files the result only under the binary's content; after the
// write-back the run links nothing. The steps that assert go's own behavior
// are what tells a toolchain that writes the result back itself: the tool is
// then done for.
//
// The test reads two environment variables go computes, one go sets in its
// own process and one it does not, so the write-back has to hash them the
// way go did. It runs against a cache of its own: the machine's is left as
// it is.
func TestWritebackFilesTheResultUnderTheLinkKey(t *testing.T) {
	tool := buildTool(t)
	tests := []struct {
		name  string
		flags []string
	}{
		{name: "without_test_flags"},
		{name: "short", flags: []string{"-short"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOCACHE", t.TempDir())
			module := writeOrphanModule(t)
			t.Chdir(module)
			// run runs go test with the flags of the case first.
			run := func(args ...string) string {
				return goTest(t, module, append(slices.Clone(tt.flags), args...)...)
			}

			// A first run runs the test and files its result under both keys.
			record := t.TempDir()
			out := run("-toolexec", tool+" record-link "+record, "./pkg/")
			require.NotContains(t, out, "(cached)")
			records, err := readRecords(record)
			require.NoError(t, err)
			require.Len(t, records, 1, "the test binary was linked once")
			require.Equal(t, "example.com/orphan/pkg", records[0].importPath)
			require.Equal(t, []string{recordFile}, entryNames(t, record), "the record holds no binary, only their build IDs")

			// A second run finds the result by the link key and links nothing.
			record = t.TempDir()
			out = run("-toolexec", tool+" record-link "+record, "./pkg/")
			require.Contains(t, out, "(cached)")
			_, err = os.Stat(filepath.Join(record, recordFile))
			require.ErrorIs(t, err, fs.ErrNotExist, "a result found by the link key skips the link")

			// A function the test never reaches changes the link key and not
			// the binary: the binary is linked, the result is found by its
			// content, and go files nothing under the new link key, so the
			// next run links again.
			writeFile(t, filepath.Join(module, "dep", "unused.go"), "package dep\n\nfunc Unused() int { return 2 }\n")
			record = t.TempDir()
			out = run("-toolexec", tool+" record-link "+record, "./pkg/")
			require.Contains(t, out, "(cached)")
			records, err = readRecords(record)
			require.NoError(t, err)
			require.Len(t, records, 1, "the orphaned binary was linked again")
			require.Equal(t, 1, linksIn(run("-x", "./pkg/")), "and is linked on every run until the result is filed under the link key")

			cmd := exec.Command(tool, append([]string{"writeback", record}, tt.flags...)...)
			cmd.Dir = module
			out2, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out2)

			out = run("-x", "./pkg/")
			require.Contains(t, out, "(cached)")
			require.Equal(t, 0, linksIn(out), "the written-back result is found by the link key")
		})
	}
}

// TestKeyedTestArgs pins the test args keyedTestArgs derives from the flags of
// a run, the ones go 1.27 keyed the result of each run by (GODEBUG
// gocachehash=1 prints them), the two examples of its doc comment first.
func TestKeyedTestArgs(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  []string
	}{
		{"a_build_flag_and_short", []string{"-race", "-short"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.short=true"}},
		{"short_then_timeout", []string{"-short", "-timeout", "5m"}, []string{"-test.v=test2json", "-test.short=true", "-test.timeout=5m0s"}},
		{"no_flags", nil, []string{"-test.v=test2json", "-test.timeout=10m0s"}},
		{"timeout_in_another_unit_first", []string{"-timeout=300s", "-short"}, []string{"-test.v=test2json", "-test.timeout=5m0s", "-test.short=true"}},
		{"bool_value_printed_back", []string{"-short=0"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.short=false"}},
		{"other_spellings_of_a_flag", []string{"--short", "-test.failfast"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.short=true", "-test.failfast=true"}},
		{"strings_in_the_order_given", []string{"-skip=TestY", "-run", "TestX"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.skip=TestY", "-test.run=TestX"}},
		{"v_beside_the_one_json_injects", []string{"-v", "-fullpath"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.v=true", "-test.fullpath=true"}},
		{"build_flags_and_their_values_left_out", []string{"-gcflags=all=-dwarf=false", "-tags", "sample", "-short"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.short=true"}},
		{"flags_go_keeps_out_of_the_key_left_out", []string{"-coverprofile=c.out", "-outputdir", "out", "-short"}, []string{"-test.v=test2json", "-test.timeout=10m0s", "-test.short=true"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := keyedTestArgs(tt.flags)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

// TestKeyedTestArgsRefusesWhatItCannotRead pins the flags keyedTestArgs has
// no key for: a key computed from them would match nothing go wrote.
func TestKeyedTestArgsRefusesWhatItCannotRead(t *testing.T) {
	for _, flags := range [][]string{{"-timeout=soon"}, {"-short=maybe"}, {"-run"}, {"./pkg/"}} {
		_, err := keyedTestArgs(flags)
		require.Error(t, err, "%q", flags)
	}
}

// TestGoEnvLeavesOutWhatGoReportsWithoutSetting pins the environment the
// write-back aligns with go's: what go computes and sets in its process is
// in, what it computes for the report alone is out.
func TestGoEnvLeavesOutWhatGoReportsWithoutSetting(t *testing.T) {
	env, err := goEnv()
	require.NoError(t, err)

	for _, name := range []string{"GOARCH", "GOCACHE", "GOPROXY", "GOROOT"} {
		require.Contains(t, env, name)
	}
	for _, name := range []string{"GOMOD", "GOWORK", "GOBIN", "CGO_CFLAGS", "GOGCCFLAGS"} {
		require.NotContains(t, env, name)
	}
}

// buildTool builds the command under test into a temporary directory.
func buildTool(t *testing.T) string {
	t.Helper()
	tool := filepath.Join(t.TempDir(), "testcachefix")
	cmd := exec.Command("go", "build", "-o", tool, ".")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return tool
}

// writeOrphanModule writes a module whose package pkg tests a function of
// package dep, the shape an orphaned test binary needs: a change to dep
// that pkg's test does not reach.
func writeOrphanModule(t *testing.T) string {
	t.Helper()
	module := t.TempDir()
	writeFile(t, filepath.Join(module, "go.mod"), "module example.com/orphan\n\ngo 1.27\n")
	writeFile(t, filepath.Join(module, "dep", "dep.go"), "package dep\n\nfunc Used() int { return 1 }\n")
	writeFile(t, filepath.Join(module, "pkg", "pkg.go"), "package pkg\n")
	writeFile(t, filepath.Join(module, "pkg", "pkg_test.go"), `package pkg

import (
	"os"
	"testing"

	"example.com/orphan/dep"
)

func TestUsed(t *testing.T) {
	// GOPROXY is computed by go and set in its own process, GOMOD computed
	// and reported only; a test that reads either puts it in its input list.
	t.Setenv("GOPROXY", "off")
	_ = os.Getenv("GOMOD")
	if dep.Used() != 1 {
		t.Fatal("dep.Used() != 1")
	}
}
`)
	return module
}

// entryNames returns the names of the entries of the directory dir.
func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// goTest runs go test -json in the module with args and returns its JSON
// stream, the form make test consumes and keys results on.
func goTest(t *testing.T, module string, args ...string) string {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test", "-json"}, args...)...)
	cmd.Dir = module
	out, err := cmd.Output()
	require.NoError(t, err, "%s", out)
	return string(out)
}

// linksIn counts the test binaries a go test -x -json stream linked.
func linksIn(out string) int {
	return strings.Count(out, "link -o ")
}
