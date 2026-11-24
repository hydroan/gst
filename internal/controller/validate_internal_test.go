package controller

import (
	"os"
	"os/exec"
	"testing"

	"github.com/hydroan/gst/internal/gst"
	"github.com/stretchr/testify/require"
)

// freshPatchHelper marks the child process of
// TestPatchNamesTheFieldInAFreshProcess.
const freshPatchHelper = "GST_TEST_FRESH_PATCH"

// TestPatchNamesTheFieldInAFreshProcess pins that the validator is set up
// before any validation runs, at package initialization: a process whose
// first validation is a patch's, which checks the fields the body names
// alone, names the field refused all the same. The test binary runs itself
// with this test selected and the helper marked, so nothing else validates
// first.
func TestPatchNamesTheFieldInAFreshProcess(t *testing.T) {
	if os.Getenv(freshPatchHelper) == "1" {
		refused := validatePatchFields(&validatedProbe{}, patchFieldSet{"Name": {}})
		require.Error(t, refused)
		var serviceErr *gst.Error
		require.ErrorAs(t, clientSafeBindError(refused), &serviceErr)
		require.Equal(t, "name is a required field", serviceErr.Msg())
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestPatchNamesTheFieldInAFreshProcess$", "-test.v")
	cmd.Env = append(os.Environ(), freshPatchHelper+"=1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "the child process must pass:\n%s", out)
	require.Contains(t, string(out), "--- PASS: TestPatchNamesTheFieldInAFreshProcess", "the child must have run the helper:\n%s", out)
}
