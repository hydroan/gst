package gen

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
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
			want:      "common",
		},
		{
			name:      "nested_duplicate_collapses_once",
			modelFile: filepath.Join("repo", "model", "foo", "bar", "bar.go"),
			want:      filepath.Join("foo", "bar"),
		},
		{
			name:      "nested_duplicate_full_chain",
			modelFile: filepath.Join("repo", "model", "x", "x", "x.go"),
			want:      "x",
		},
		{
			name:      "stem_differs_from_parent_dir",
			modelFile: filepath.Join("repo", "model", "config", "namespace", "app", "env", "item.go"),
			want:      filepath.Join("config", "namespace", "app", "env", "item"),
		},
		{
			name:      "flat_model_file",
			modelFile: filepath.Join("repo", "model", "user.go"),
			want:      "user",
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

func TestServiceTarget(t *testing.T) {
	t.Parallel()

	modelDir := filepath.Join("repo", "model")
	serviceDir := filepath.Join("repo", "service")
	model := &ModelInfo{
		ModulePath:    "github.com/acme/app",
		ModelPkgName:  "authz",
		ModelName:     "Role",
		ModelFileDir:  filepath.Join("repo", "model", "authz"),
		ModelFilePath: filepath.Join("repo", "model", "authz", "role.go"),
	}

	tests := []struct {
		name       string
		action     *dsl.Action
		wantFile   string
		wantImport string
		wantPkg    string
	}{
		{
			name: "default nested service target",
			action: &dsl.Action{
				Enabled:  true,
				Service:  true,
				Filename: "role.go",
				Phase:    consts.PHASE_CREATE,
			},
			wantFile:   filepath.Join("repo", "service", "authz", "role", "role.go"),
			wantImport: "github.com/acme/app/repo/service/authz/role",
			wantPkg:    "role",
		},
		{
			name: "flatten service target",
			action: &dsl.Action{
				Enabled:  true,
				Service:  true,
				Filename: "role.go",
				Flatten:  true,
				Phase:    consts.PHASE_CREATE,
			},
			wantFile:   filepath.Join("repo", "service", "authz", "role.go"),
			wantImport: "github.com/acme/app/repo/service/authz",
			wantPkg:    "authz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ServiceTarget(model, tt.action, modelDir, serviceDir)
			if got.FilePath != tt.wantFile {
				t.Fatalf("FilePath = %q, want %q", got.FilePath, tt.wantFile)
			}
			if got.ImportPath != tt.wantImport {
				t.Fatalf("ImportPath = %q, want %q", got.ImportPath, tt.wantImport)
			}
			if got.PackageName != tt.wantPkg {
				t.Fatalf("PackageName = %q, want %q", got.PackageName, tt.wantPkg)
			}
		})
	}
}
