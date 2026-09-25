package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggconst"
	"github.com/stretchr/testify/require"
)

// TestGenRunImportsTheInterceptorPackageWhenPresent pins that main.go
// imports the project's interceptor package, for the init function that
// registers its interceptors, exactly when the package exists: a project
// without one, the ordinary HTTP project, keeps its main.go as it is.
func TestGenRunImportsTheInterceptorPackageWhenPresent(t *testing.T) {
	projectDir := newGenProject(t)
	writeProtobufProject(t, projectDir, map[string]string{"model/note.go": protobufNoteModel})
	interceptorFile := filepath.Join(projectDir, ggconst.DirInterceptor, "interceptor.go")
	writeProjectFile(t, interceptorFile, "package interceptor\n")

	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	mainCode, err := os.ReadFile(filepath.Join(projectDir, ggconst.FileMain))
	require.NoError(t, err)
	require.Contains(t, string(mainCode), `_ "tmpapp/interceptor"`)

	require.NoError(t, os.RemoveAll(filepath.Dir(interceptorFile)))
	require.NoError(t, genRunWithOptions(genRunOptions{Quiet: true}))
	mainCode, err = os.ReadFile(filepath.Join(projectDir, ggconst.FileMain))
	require.NoError(t, err)
	require.NotContains(t, string(mainCode), "tmpapp/interceptor", "without the package main.go must not import it")
}
