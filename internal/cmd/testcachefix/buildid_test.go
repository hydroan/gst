package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestContentIDMatchesGoTool pins the content ID on the toolchain at hand:
// for a test binary go linked and finalized, the hash of the file with its
// build ID zeroed is the last part of the build ID go tool buildid prints.
// The host's binary and an ELF one each exercise their own exclusions, the
// Mach-O code signature and UUID on darwin, the GNU build ID note on linux.
func TestContentIDMatchesGoTool(t *testing.T) {
	t.Setenv("GOCACHE", t.TempDir())
	module := writeOrphanModule(t)

	for _, target := range []struct{ name, goos, goarch string }{
		{"host", runtime.GOOS, runtime.GOARCH},
		{"elf", "linux", "amd64"},
	} {
		t.Run(target.name, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), "pkg.test")
			cmd := exec.Command("go", "test", "-c", "-o", binary, "./pkg/")
			cmd.Dir = module
			cmd.Env = append(os.Environ(), "GOOS="+target.goos, "GOARCH="+target.goarch)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", out)

			out, err = exec.Command("go", "tool", "buildid", binary).Output()
			require.NoError(t, err)
			buildID := strings.TrimSpace(string(out))
			parts := strings.Split(buildID, "/")
			require.Len(t, parts, 4)

			id, err := contentID(binary, buildID)
			require.NoError(t, err)
			require.Equal(t, parts[3], id)
		})
	}
}
