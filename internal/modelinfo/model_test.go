package modelinfo_test

import (
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
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
			modelFile: filepath.Join("repo", "model", "sample", "sample.go"),
			want:      "sample",
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
			got := modelinfo.ServiceOutputRel(tt.modelFile, modelDir)
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
	model := &modelinfo.Model{
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
				Service:     true,
				ServiceName: "role",
				Phase:       consts.Create,
			},
			wantFile:   filepath.Join("repo", "service", "authz", "role", "role.go"),
			wantImport: "github.com/acme/app/repo/service/authz/role",
			wantPkg:    "role",
		},
		{
			name: "flatten_service_target",
			action: &dsl.Action{
				Service:     true,
				ServiceName: "role",
				Flatten:     true,
				Phase:       consts.Create,
			},
			wantFile:   filepath.Join("repo", "service", "authz", "role.go"),
			wantImport: "github.com/acme/app/repo/service/authz",
			wantPkg:    "authz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := modelinfo.ServiceTarget(model, tt.action, modelDir, serviceDir)
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

func TestModelPackageName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dirName string
		want    string
	}{
		{name: "plain_directory", dirName: "sample", want: "sample"},
		{name: "underscored_directory", dirName: "record_item", want: "recorditem"},
		{name: "root_model_directory", dirName: "model", want: "model"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := modelinfo.ModelPackageName(tt.dirName); got != tt.want {
				t.Errorf("ModelPackageName(%q) = %q, want %q", tt.dirName, got, tt.want)
			}
		})
	}
}

func TestModel_InModelRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		modelFileDir string
		want         bool
	}{
		{
			name:         "root_model_package",
			modelFileDir: "model",
			want:         true,
		},
		{
			name:         "sub_package",
			modelFileDir: "model/sample",
			want:         false,
		},
		{
			// A package named model below the root is still a sub package:
			// the root is told apart by its directory, not by its name.
			name:         "sub_package_named_model",
			modelFileDir: "model/sample/model",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			m := &modelinfo.Model{ModulePath: "helloworld", ModelFileDir: tt.modelFileDir}
			if got := m.InModelRoot("model"); got != tt.want {
				t.Errorf("InModelRoot(%q) = %v, want %v", "model", got, tt.want)
			}
		})
	}
}

// TestPackageImportPath pins the examples of PackageImportPath's doc comment
// and that a model's ImportPath is the same rule applied to its directory.
func TestPackageImportPath(t *testing.T) {
	for _, tt := range []struct{ dir, want string }{
		{"model/sample", "tmpapp/model/sample"},
		{"model/", "tmpapp/model"},
		{"", "tmpapp"},
		{".", "tmpapp"},
	} {
		if got := modelinfo.PackageImportPath("tmpapp", tt.dir); got != tt.want {
			t.Fatalf("PackageImportPath(tmpapp, %q) = %q, want %q", tt.dir, got, tt.want)
		}
	}
	m := &modelinfo.Model{ModulePath: "tmpapp", ModelFileDir: "model/sample"}
	if got := m.ImportPath(); got != "tmpapp/model/sample" {
		t.Fatalf("ImportPath() = %q, want tmpapp/model/sample", got)
	}
}
