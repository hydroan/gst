package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestGenRunImportsTheInterceptorPackageWhenPresent pins that main.go, and
// every TestMain file of an external test package with it, imports the
// project's interceptor package, for the init function that registers its
// interceptors, exactly when the package exists: a project without one, the
// ordinary HTTP project, keeps its main.go and its TestMain files as they
// are.
func TestGenRunImportsTheInterceptorPackageWhenPresent(t *testing.T) {
	projectDir, ok := newGenProject(t)
	if !ok {
		return
	}
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	interceptorFile := filepath.Join(projectDir, ggconst.DirInterceptor, "interceptor.go")
	writeProjectFile(t, interceptorFile, "package interceptor\n")
	testMain := filepath.Join(projectDir, "service/note/main_test.go")
	writeProjectFile(t, testMain, "package note_test\n\nimport \"testing\"\n\nfunc TestMain(m *testing.M) {}\n")

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	for _, path := range []string{filepath.Join(projectDir, ggconst.FileMain), testMain} {
		code, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Contains(t, string(code), `_ "tmpapp/interceptor"`, path)
	}

	require.NoError(t, os.RemoveAll(filepath.Dir(interceptorFile)))
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	for _, path := range []string{filepath.Join(projectDir, ggconst.FileMain), testMain} {
		code, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotContains(t, string(code), "tmpapp/interceptor", "without the package %s must not import it", path)
	}
}
