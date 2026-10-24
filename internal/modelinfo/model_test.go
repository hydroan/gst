package modelinfo_test

import (
	"path/filepath"
	"slices"
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

// TestServiceConflictsNamesBothActionsOfAService pins the examples of
// ServiceConflicts and the forms gg gen would write into a project that does
// not build: an action flattened onto the file of another model's service,
// two files of one package flattened onto one file, a model file and the
// package named after it collapsed onto one directory, two models of one
// file each with a service, two route actions naming one file, an explicit
// name meeting a default one, two flattened actions of one name, a default
// type named by another action, a type named alike in case and underscore,
// and an action sharing its file with one action and its type with
// another; a flattened and an unflattened action of one name, distinct names
// and actions without a service conflict with nothing.
func TestServiceConflictsNamesBothActionsOfAService(t *testing.T) {
	model := func(pkg, name, design string) string {
		return "package " + pkg + `

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type ` + name + ` struct {
	model.Base
}

func (` + name + `) Design() {
` + design + `}
`
	}
	twoModels := func(pkg, first, firstDesign, second, secondDesign string) string {
		return model(pkg, first, firstDesign) + `
type ` + second + ` struct {
	model.Base
}

func (` + second + `) Design() {
` + secondDesign + `}
`
	}
	record := filepath.Join("model", "sample", "record.go")
	tests := []struct {
		name    string
		sources map[string]string
		want    []string
	}{
		{
			name: "an action flattened onto the file of another model's service",
			sources: map[string]string{
				filepath.Join("model", "record.go"):         model("model", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Create(func() {\n\t\tdsl.Service()\n\t})\n"),
				filepath.Join("model", "record", "item.go"): model("record", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.Create(func() {\n\t\tdsl.Flatten()\n\t\tdsl.Service(\"create\")\n\t})\n"),
			},
			want: []string{`model/record.go: the Create action of Record writes its service to service/record/create.go, as the Create action of Item in model/record/item.go does; a service file serves one action, so give each a Service("name") of its own`},
		},
		{
			name: "two files of one package flattened onto one file",
			sources: map[string]string{
				filepath.Join("model", "authz", "role.go"):    model("authz", "Role", "\tdsl.Endpoint(\"roles\")\n\tdsl.Create(func() {\n\t\tdsl.Flatten()\n\t\tdsl.Service(\"grant\")\n\t})\n"),
				filepath.Join("model", "authz", "binding.go"): model("authz", "Binding", "\tdsl.Endpoint(\"bindings\")\n\tdsl.Create(func() {\n\t\tdsl.Flatten()\n\t\tdsl.Service(\"grant\")\n\t})\n"),
			},
			want: []string{`model/authz/role.go: the Create action of Role writes its service to service/authz/grant.go, as the Create action of Binding in model/authz/binding.go does; a service file serves one action, so give each a Service("name") of its own`},
		},
		{
			name: "a model file and the package named after it collapsed onto one directory",
			sources: map[string]string{
				filepath.Join("model", "archive.go"):            model("model", "Box", "\tdsl.Endpoint(\"boxes\")\n\tdsl.Get(func() {\n\t\tdsl.Service()\n\t})\n"),
				filepath.Join("model", "archive", "archive.go"): model("archive", "Archive", "\tdsl.Endpoint(\"archives\")\n\tdsl.Create(func() {\n\t\tdsl.Service()\n\t})\n"),
			},
			want: []string{`model/archive.go: the Get action of Box writes package box into service/archive, where the Create action of Archive in model/archive/archive.go writes package archive; a directory holds one package`},
		},
		{
			name: "two models of one file each with a service",
			sources: map[string]string{
				record: twoModels("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Get(func() {\n\t\tdsl.Service()\n\t})\n", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.List(func() {\n\t\tdsl.Service()\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the List action of Item writes package item into service/sample/record, where the Get action of Record in model/sample/record.go writes package record; a directory holds one package`},
		},
		{
			name: "two route actions naming one file",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/detail\", func() {\n\t\tdsl.Get(func() {\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/list\", func() {\n\t\tdsl.List(func() {\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the List action of Record under Route("sample/list") writes its service to service/sample/record/shared.go, as the Get action of Record under Route("sample/detail") in model/sample/record.go does; a service file serves one action, so give each a Service("name") of its own`},
		},
		{
			name: "an explicit name meeting a default one",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Get(func() {\n\t\tdsl.Service()\n\t})\n\tdsl.Route(\"sample/detail\", func() {\n\t\tdsl.Patch(func() {\n\t\t\tdsl.Service(\"get\")\n\t\t})\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the Patch action of Record under Route("sample/detail") writes its service to service/sample/record/get.go, as the Get action of Record in model/sample/record.go does; a service file serves one action, so give each a Service("name") of its own`},
		},
		{
			name: "two models of one file meeting at the default file",
			sources: map[string]string{
				record: twoModels("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Get(func() {\n\t\tdsl.Service()\n\t})\n", "Item", "\tdsl.Endpoint(\"items\")\n\tdsl.Get(func() {\n\t\tdsl.Service()\n\t})\n"),
			},
			want: []string{
				`model/sample/record.go: the Get action of Item writes its service to service/sample/record/get.go, as the Get action of Record in model/sample/record.go does; a service file serves one action, so give each a Service("name") of its own`,
				`model/sample/record.go: the Get action of Item writes package item into service/sample/record, where the Get action of Record in model/sample/record.go writes package record; a directory holds one package`,
			},
		},
		{
			name: "two flattened actions of one name",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/archive\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Flatten()\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/restore\", func() {\n\t\tdsl.Update(func() {\n\t\t\tdsl.Flatten()\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the Update action of Record under Route("sample/restore") writes its service to service/sample/shared.go, as the Create action of Record under Route("sample/archive") in model/sample/record.go does; a service file serves one action, so give each a Service("name") of its own`},
		},
		{
			name: "a flattened and an unflattened action of one name",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/archive\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Flatten()\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/restore\", func() {\n\t\tdsl.Update(func() {\n\t\t\tdsl.Service(\"shared\")\n\t\t})\n\t})\n"),
			},
		},
		{
			name: "distinct names",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/detail\", func() {\n\t\tdsl.Get(func() {\n\t\t\tdsl.Service(\"detail\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/list\", func() {\n\t\tdsl.List(func() {\n\t\t\tdsl.Service(\"list\")\n\t\t})\n\t})\n"),
			},
		},
		{
			name: "actions without a service",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/detail\", func() {\n\t\tdsl.Get(func() {})\n\t})\n\tdsl.Route(\"sample/list\", func() {\n\t\tdsl.Get(func() {})\n\t})\n"),
			},
		},
		{
			name: "a default type named by another action",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Create(func() {\n\t\tdsl.Service()\n\t})\n\tdsl.Update(func() {\n\t\tdsl.Service(\"creator\")\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the Update action of Record declares the service type Creator in service/sample/record, as the Create action of Record in model/sample/record.go does; a package declares a name once, so give each a Service("name") of its own`},
		},
		{
			name: "a type named alike in case and underscore",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/archive\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Service(\"item_archive\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/seal\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Service(\"itemArchive\")\n\t\t})\n\t})\n"),
			},
			want: []string{`model/sample/record.go: the Create action of Record under Route("sample/seal") declares the service type ItemArchive in service/sample/record, as the Create action of Record under Route("sample/archive") in model/sample/record.go does; a package declares a name once, so give each a Service("name") of its own`},
		},
		{
			name: "an action sharing its file with one action and its type with another",
			sources: map[string]string{
				record: model("sample", "Record", "\tdsl.Endpoint(\"records\")\n\tdsl.Route(\"sample/seal\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Service(\"itemArchive\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/lock\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Service(\"itemarchive\")\n\t\t})\n\t})\n\tdsl.Route(\"sample/archive\", func() {\n\t\tdsl.Create(func() {\n\t\t\tdsl.Service(\"item_archive\")\n\t\t})\n\t})\n"),
			},
			want: []string{
				`model/sample/record.go: the Create action of Record under Route("sample/seal") writes its service to service/sample/record/itemarchive.go, as the Create action of Record under Route("sample/lock") in model/sample/record.go does; a service file serves one action, so give each a Service("name") of its own`,
				`model/sample/record.go: the Create action of Record under Route("sample/seal") declares the service type ItemArchive in service/sample/record, as the Create action of Record under Route("sample/archive") in model/sample/record.go does; a package declares a name once, so give each a Service("name") of its own`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, err := range modelinfo.ServiceConflicts(findModels(t, tt.sources)) {
				got = append(got, err.Error())
			}

			if !slices.Equal(got, tt.want) {
				t.Fatalf("ServiceConflicts = %q, want %q", got, tt.want)
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
