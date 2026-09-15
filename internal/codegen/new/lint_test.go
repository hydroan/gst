//nolint:predeclared
package new

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestExamplesCarryTheScaffoldLintConfiguration proves the example projects
// lint with the configuration gg new writes, byte for byte: they stand in
// for a generated project, so a rule that changes here changes there.
func TestExamplesCarryTheScaffoldLintConfiguration(t *testing.T) {
	for _, example := range []string{"demo", "cluster", "bench"} {
		t.Run(example, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", example, ".golangci.yml"))
			require.NoError(t, err)
			require.Equal(t, golangciLintContent, string(content))
		})
	}
}
