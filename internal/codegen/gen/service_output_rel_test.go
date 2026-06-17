package gen

import (
	"path/filepath"
	"testing"
)

func TestServiceOutputRel(t *testing.T) {
	t.Parallel()
	modelDir := filepath.Join("repo", "model")

	tests := []struct {
		name      string
		modelFile string
		want      string
	}{
		{
			name:      "duplicate_dir_and_stem",
			modelFile: filepath.Join("repo", "model", "common", "common.go"),
			want:      filepath.Join("common"),
		},
		{
			name:      "nested_duplicate_collapses_once",
			modelFile: filepath.Join("repo", "model", "foo", "bar", "bar.go"),
			want:      filepath.Join("foo", "bar"),
		},
		{
			name:      "nested_duplicate_full_chain",
			modelFile: filepath.Join("repo", "model", "x", "x", "x.go"),
			want:      filepath.Join("x"),
		},
		{
			name:      "stem_differs_from_parent_dir",
			modelFile: filepath.Join("repo", "model", "config", "namespace", "app", "env", "item.go"),
			want:      filepath.Join("config", "namespace", "app", "env", "item"),
		},
		{
			name:      "flat_model_file",
			modelFile: filepath.Join("repo", "model", "user.go"),
			want:      filepath.Join("user"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := ServiceOutputRel(tt.modelFile, modelDir)
			if got != tt.want {
				t.Fatalf("ServiceOutputRel(%q, %q) = %q, want %q", tt.modelFile, modelDir, got, tt.want)
			}
		})
	}
}
