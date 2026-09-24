package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/stretchr/testify/require"
)

// TestGenerateServiceTest compares the whole file GenerateServiceTest builds
// for the example of its doc comment: the List action of the model Record,
// registered under records.
func TestGenerateServiceTest(t *testing.T) {
	target := gen.ServiceTargetInfo{
		Dir:         "service/record",
		FilePath:    "service/record/list.go",
		ImportPath:  "helloworld/service/record",
		PackageName: "record",
	}
	action := &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_LIST}

	got, err := gen.GenerateServiceTest(target, action, "records")
	require.NoError(t, err)
	want := `package record_test

import "testing"

// TestList covers GET /api/records, served by Lister in list.go.
func TestList(t *testing.T) {
	t.Fatal("TestList is a scaffold: replace it with the test of GET /api/records")
}
`
	require.Equal(t, want, got)
}

// TestGenerateServiceTestNamesTheAction pins how the scaffold names the test
// and the route for the other action shapes: the phase names a default
// action, the file name names a Filename action, and a flattened file takes
// the package of its model.
func TestGenerateServiceTestNamesTheAction(t *testing.T) {
	tests := []struct {
		name        string
		target      gen.ServiceTargetInfo
		action      *dsl.Action
		route       string
		wantPackage string
		wantDoc     string
	}{
		{
			name:        "get_names_the_parameter_of_its_route",
			target:      gen.ServiceTargetInfo{Dir: "service/record", FilePath: "service/record/get.go", PackageName: "record"},
			action:      &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_GET},
			route:       "records/:rec",
			wantPackage: "record_test",
			wantDoc:     "// TestGet covers GET /api/records/:rec, served by Getter in get.go.",
		},
		{
			name:        "delete_many_is_a_delete_of_the_batch_route",
			target:      gen.ServiceTargetInfo{Dir: "service/record", FilePath: "service/record/delete_many.go", PackageName: "record"},
			action:      &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_DELETE_MANY},
			route:       "records/batch",
			wantPackage: "record_test",
			wantDoc:     "// TestDeleteMany covers DELETE /api/records/batch, served by ManyDeleter in delete_many.go.",
		},
		{
			name:        "sse_is_a_get",
			target:      gen.ServiceTargetInfo{Dir: "service/notice", FilePath: "service/notice/sse.go", PackageName: "notice"},
			action:      &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_SSE},
			route:       "notices",
			wantPackage: "notice_test",
			wantDoc:     "// TestSSE covers GET /api/notices, served by Streamer in sse.go.",
		},
		{
			name:        "filename_action_is_named_after_its_file",
			target:      gen.ServiceTargetInfo{Dir: "service/record", FilePath: "service/record/archive.go", PackageName: "record"},
			action:      &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_CREATE, Filename: "archive"},
			route:       "records/archive",
			wantPackage: "record_test",
			wantDoc:     "// TestArchive covers POST /api/records/archive, served by Archive in archive.go.",
		},
		{
			name:        "flattened_file_takes_the_package_of_its_model",
			target:      gen.ServiceTargetInfo{Dir: "service/archive", FilePath: "service/archive/seal.go", PackageName: "archive"},
			action:      &dsl.Action{Enabled: true, Service: true, Phase: consts.PHASE_CREATE, Filename: "seal", Flatten: true},
			route:       "archive/documents/seal",
			wantPackage: "archive_test",
			wantDoc:     "// TestSeal covers POST /api/archive/documents/seal, served by Seal in seal.go.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := gen.GenerateServiceTest(tt.target, tt.action, tt.route)
			require.NoError(t, err)
			require.True(t, strings.HasPrefix(got, "package "+tt.wantPackage+"\n"), got)
			require.Contains(t, got, tt.wantDoc+"\n")
		})
	}
}

// TestGenerateServiceTestMain compares the whole file GenerateServiceTestMain
// builds for the example of its doc comment: the service package record of
// the module helloworld.
func TestGenerateServiceTestMain(t *testing.T) {
	got, err := gen.GenerateServiceTestMain("helloworld", "record")
	require.NoError(t, err)
	want := `package record_test

import (
	"testing"

	// The registrations of main.go: the models, modules, services and cron
	// jobs register themselves through the init of these packages.
	_ "helloworld/component"
	_ "helloworld/configx"
	_ "helloworld/cronjob"
	_ "helloworld/leader"
	_ "helloworld/lock"
	_ "helloworld/middleware"
	_ "helloworld/model"
	_ "helloworld/module"
	"helloworld/router"
	_ "helloworld/service"

	"github.com/hydroan/gst/testutil"
)

// TestMain starts the test server of this package the way main.go starts the
// application. Declare what the tests need on the Server, such as Database
// or Redis.
func TestMain(m *testing.M) {
	testutil.Run(m, testutil.Server{
		Routes: router.Init,
	})
}
`
	require.Equal(t, want, got)
}

// TestPackageDeclaresTestMain pins where PackageDeclaresTestMain looks for a
// TestMain: in any test file of the directory, main_test.go or not, and only
// at package-level functions.
func TestPackageDeclaresTestMain(t *testing.T) {
	testMain := "package record_test\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {}\n"

	t.Run("no_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create.go"), "package record\n")

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("main_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "main_test.go"), testMain)

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.True(t, declared)
	})
	t.Run("test_main_in_a_paired_test_file", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), testMain)

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.True(t, declared)
	})
	t.Run("a_method_named_test_main_does_not_count", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), "package record_test\n\nimport \"testing\"\n\ntype suite struct{}\n\nfunc (suite) TestMain(m *testing.M) {}\n")

		declared, err := gen.PackageDeclaresTestMain(dir)
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("missing_directory", func(t *testing.T) {
		declared, err := gen.PackageDeclaresTestMain(filepath.Join(t.TempDir(), "missing"))
		require.NoError(t, err)
		require.False(t, declared)
	})
	t.Run("unparsable_test_file_is_an_error", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "create_test.go"), "package record_test\n\nfunc {\n")

		_, err := gen.PackageDeclaresTestMain(dir)
		require.Error(t, err)
	})
}

// writeFile writes content to path.
func writeFile(t *testing.T, path, content string) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}
