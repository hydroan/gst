package gen_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
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
			modelFile: filepath.Join("repo", "model", "sample", "record", "entry", "detail", "item.go"),
			want:      filepath.Join("sample", "record", "entry", "detail", "item"),
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
			got := gen.ServiceOutputRel(tt.modelFile, modelDir)
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
	model := &gen.ModelInfo{
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
			name: "default_nested_service_target",
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
			name: "flatten_service_target",
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

			got := gen.ServiceTarget(model, tt.action, modelDir, serviceDir)
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

func TestModelInfo_ModelImportPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		modelFileDir string
		wantPath     string
		wantImport   bool
	}{
		{
			// The root model package is the package the generated model
			// registration file lives in, so it is never imported there.
			name:         "root_model_package",
			modelFileDir: "model",
			wantPath:     "",
			wantImport:   false,
		},
		{
			name:         "sub_package",
			modelFileDir: "model/sample",
			wantPath:     "helloworld/model/sample",
			wantImport:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &gen.ModelInfo{ModulePath: "helloworld", ModelFileDir: tt.modelFileDir}
			gotPath, gotImport := m.ModelImportPath()
			if gotPath != tt.wantPath || gotImport != tt.wantImport {
				t.Errorf("ModelImportPath() = (%q, %v), want (%q, %v)", gotPath, gotImport, tt.wantPath, tt.wantImport)
			}
		})
	}
}
