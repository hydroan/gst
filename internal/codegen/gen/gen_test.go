package gen

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
	"github.com/kr/pretty"
	_ "github.com/sergi/go-diff/diffmatchpatch"
)

var src1 = `
package model

import "github.com/hydroan/gst/model"

type User struct {
	Name  string
	Age   int
	Email string

	model.Base
}

type Group struct {
	Name    string
	Members []User

	model.Base
}

type GroupUser struct {
	GroupId int
	UserId  int
}

type Device struct {
	Name string

	model.AutoBase
}
	`

var src2 = `
package model

import model_auth "github.com/hydroan/gst/model"

type User struct {
	Name  string
	Age   int
	Email string

	model_auth.Base
}

type Group struct {
	Name    string
	Members []User

	model_auth.Base
}

type GroupUser struct {
	GroupId int
	UserId  int
}

type Device struct {
	Name string

	model_auth.AutoBase
}
	`

var dataServiceUserCreate string

func init() {
	var data []byte
	var err error
	if data, err = os.ReadFile("./testdata/service/user_create.go"); err != nil {
		panic(err)
	}
	dataServiceUserCreate = string(data)
}

func TestGetModulePath(t *testing.T) {
	content := []byte("module github.com/hydroan/gst")
	if err := os.WriteFile("go.mod", content, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("go.mod")

	tests := []struct {
		name    string // description of this test case
		want    string
		wantErr bool
	}{
		{
			name:    "test1",
			want:    "github.com/hydroan/gst",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := GetModulePath()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("getModulePath() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("getModulePath() succeeded unexpectedly")
			}
			if got != tt.want {
				t.Errorf("getModulePath() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetModulePathInWorkspaceReturnsCurrentModuleOnly(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	libDir := filepath.Join(dir, "lib")
	for moduleDir, moduleName := range map[string]string{appDir: "example.com/app", libDir: "example.com/lib"} {
		if err := os.MkdirAll(moduleDir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module "+moduleName+"\n\ngo 1.24\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	goWork := filepath.Join(dir, "go.work")
	if err := os.WriteFile(goWork, []byte("go 1.24\n\nuse (\n\t./app\n\t./lib\n)\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(appDir)
	t.Setenv("GOWORK", goWork)

	got, err := GetModulePath()
	if err != nil {
		t.Fatalf("GetModulePath() error = %v", err)
	}
	if got != "example.com/app" {
		t.Fatalf("GetModulePath() = %q, want %q (workspace mode must not leak other modules)", got, "example.com/app")
	}
}

func TestFindModelPackageName(t *testing.T) {
	fset := token.NewFileSet()
	file1, err := parser.ParseFile(fset, "user.go", src1, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	file2, err := parser.ParseFile(fset, "user.go", src2, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		file *ast.File
		want string
	}{
		{
			name: "default",
			file: file1,
			want: "model",
		},
		{
			name: "named",
			file: file2,
			want: "model",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findModelPackageName(tt.file)
			if got != tt.want {
				t.Errorf("findModelPackageName() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFindModels(t *testing.T) {
	content := []byte("module github.com/hydroan/gst")
	if err := os.WriteFile("go.mod", content, 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove("go.mod")

	modulePath, err := GetModulePath()
	if err != nil {
		t.Fatal(err)
	}

	tmpdir := "/tmp/model"
	if err = os.MkdirAll(tmpdir, 0o750); err != nil {
		t.Fatal(err)
	}

	filename1 := filepath.Join(tmpdir, "user.go")
	filename2 := filepath.Join(tmpdir, "user2.go")
	if err = os.WriteFile(filename1, []byte(src1), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filename2, []byte(src2), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		modulePath string
		modelPath  string
		filename   string
		want       []*ModelInfo
		wantErr    bool
	}{
		{
			name:       "default",
			modulePath: modulePath,
			modelPath:  tmpdir,
			filename:   filename1,
			want: []*ModelInfo{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "User",
					ModelVarName:  "u",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "users",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*User", Result: "*User"},
						Delete:     &dsl.Action{Payload: "*User", Result: "*User"},
						Update:     &dsl.Action{Payload: "*User", Result: "*User"},
						Patch:      &dsl.Action{Payload: "*User", Result: "*User"},
						List:       &dsl.Action{Payload: "*User", Result: "*User"},
						Get:        &dsl.Action{Payload: "*User", Result: "*User"},
						CreateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						DeleteMany: &dsl.Action{Payload: "*User", Result: "*User"},
						UpdateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						PatchMany:  &dsl.Action{Payload: "*User", Result: "*User"},
						Import:     &dsl.Action{Payload: "*User", Result: "*User"},
						Export:     &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "Group",
					ModelVarName:  "g",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "groups",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Delete:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Update:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Patch:      &dsl.Action{Payload: "*Group", Result: "*Group"},
						List:       &dsl.Action{Payload: "*Group", Result: "*Group"},
						Get:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						CreateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						DeleteMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						UpdateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						PatchMany:  &dsl.Action{Payload: "*Group", Result: "*Group"},
						Import:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Export:     &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename1,
					ModelPkgName:  "model",
					ModelName:     "Device",
					ModelVarName:  "d",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "devices",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Delete:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Update:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Patch:      &dsl.Action{Payload: "*Device", Result: "*Device"},
						List:       &dsl.Action{Payload: "*Device", Result: "*Device"},
						Get:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						CreateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						DeleteMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						UpdateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						PatchMany:  &dsl.Action{Payload: "*Device", Result: "*Device"},
						Import:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Export:     &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:       "named",
			modulePath: modulePath,
			modelPath:  tmpdir,
			filename:   filename2,
			want: []*ModelInfo{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "User",
					ModelVarName:  "u",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "users",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*User", Result: "*User"},
						Delete:     &dsl.Action{Payload: "*User", Result: "*User"},
						Update:     &dsl.Action{Payload: "*User", Result: "*User"},
						Patch:      &dsl.Action{Payload: "*User", Result: "*User"},
						List:       &dsl.Action{Payload: "*User", Result: "*User"},
						Get:        &dsl.Action{Payload: "*User", Result: "*User"},
						CreateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						DeleteMany: &dsl.Action{Payload: "*User", Result: "*User"},
						UpdateMany: &dsl.Action{Payload: "*User", Result: "*User"},
						PatchMany:  &dsl.Action{Payload: "*User", Result: "*User"},
						Import:     &dsl.Action{Payload: "*User", Result: "*User"},
						Export:     &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "Group",
					ModelVarName:  "g",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "groups",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Delete:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Update:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Patch:      &dsl.Action{Payload: "*Group", Result: "*Group"},
						List:       &dsl.Action{Payload: "*Group", Result: "*Group"},
						Get:        &dsl.Action{Payload: "*Group", Result: "*Group"},
						CreateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						DeleteMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						UpdateMany: &dsl.Action{Payload: "*Group", Result: "*Group"},
						PatchMany:  &dsl.Action{Payload: "*Group", Result: "*Group"},
						Import:     &dsl.Action{Payload: "*Group", Result: "*Group"},
						Export:     &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  tmpdir,
					ModelFilePath: filename2,
					ModelPkgName:  "model",
					ModelName:     "Device",
					ModelVarName:  "d",
					Design: &dsl.Design{
						Enabled:    true,
						Endpoint:   "devices",
						Migrate:    false,
						Create:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Delete:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Update:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Patch:      &dsl.Action{Payload: "*Device", Result: "*Device"},
						List:       &dsl.Action{Payload: "*Device", Result: "*Device"},
						Get:        &dsl.Action{Payload: "*Device", Result: "*Device"},
						CreateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						DeleteMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						UpdateMany: &dsl.Action{Payload: "*Device", Result: "*Device"},
						PatchMany:  &dsl.Action{Payload: "*Device", Result: "*Device"},
						Import:     &dsl.Action{Payload: "*Device", Result: "*Device"},
						Export:     &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := FindModels(tt.modulePath, tt.modelPath, tt.filename)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("FindModels() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("FindModels() succeeded unexpectedly")
			}
			var got2 []ModelInfo
			var want2 []ModelInfo
			for _, v := range got {
				got2 = append(got2, *v)
			}
			for _, v := range tt.want {
				want2 = append(want2, *v)
			}
			if !reflect.DeepEqual(got2, want2) {
				t.Errorf("FindModels() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got2), pretty.Sprintf("% #v", want2))
			}
		})
	}
}

func TestModelPkg2ServicePkg(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		pkgName string
		want    string
	}{
		{
			name:    "test1",
			pkgName: "model",
			want:    "service",
		},
		{
			name:    "test2",
			pkgName: "model2",
			want:    "service2",
		},
		{
			name:    "test3",
			pkgName: "model_system",
			want:    "service_system",
		},
		{
			name:    "test4",
			pkgName: "modelAuth",
			want:    "serviceAuth",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := modelPkg2ServicePkg(tt.pkgName)
			if got != tt.want {
				t.Errorf("modelPkg2ServicePkg() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenServiceMethod1(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		want  string
		phase consts.Phase
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_CREATE_BEFORE,
			want: `func (u *Creator) CreateBefore(ctx *types.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod1(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod1() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod2(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_LIST_BEFORE,
			want: `func (u *Lister) ListBefore(ctx *types.ServiceContext, users *[]*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list before")
	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod2(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod2() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod3(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_CREATE_MANY_BEFORE,
			want: `func (u *ManyCreator) CreateManyBefore(ctx *types.ServiceContext, users ...*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create many before")
	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod3(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod3() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod4(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info    *ModelInfo
		reqName string
		rspName string
		phase   consts.Phase
		want    string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "User",
			rspName: "User",
			phase:   consts.PHASE_CREATE,
			want: `func (u *Creator) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}`,
		},
		{
			name: "group",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "Group",
				ModelVarName: "g",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "GroupRequest",
			rspName: "GroupResponse",
			phase:   consts.PHASE_UPDATE,
			want: `func (g *Updater) Update(ctx *types.ServiceContext, req model.GroupRequest) (rsp model.GroupResponse, err error) {
	log := g.WithContext(ctx, ctx.Phase())
	log.Info("group update")
	return rsp, nil
}`,
		},
		{
			name: "group2",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "Group",
				ModelVarName: "g",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "*GroupRequest",
			rspName: "*GroupResponse",
			phase:   consts.PHASE_UPDATE,
			want: `func (g *Updater) Update(ctx *types.ServiceContext, req *model.GroupRequest) (rsp *model.GroupResponse, err error) {
	log := g.WithContext(ctx, ctx.Phase())
	log.Info("group update")
	return rsp, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := genServiceMethod4(tt.info, nil, tt.reqName, tt.rspName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod4() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod5(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_IMPORT,
			want: `func (u *Importer) Import(ctx *types.ServiceContext, reader io.Reader) (users []*model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user import")
	return users, err
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod5(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod5() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod6(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_EXPORT,
			want: `func (u *Exporter) Export(ctx *types.ServiceContext, users ...*model.User) (data []byte, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user export")
	return data, err
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod6(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod5() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod7(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_FILTER,
			want: `func (u *Lister) Filter(ctx *types.ServiceContext, user *model.User) *model.User {
	return user
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := genServiceMethod7(tt.info, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(node)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod7() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenServiceMethod8(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_FILTER_RAW,
			want: `func (u *Lister) FilterRaw(ctx *types.ServiceContext) string {
	return ""
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := genServiceMethod8(tt.info, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(node)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod8() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateServiceListEmptyPayload(t *testing.T) {
	tests := []struct {
		name          string
		info          *ModelInfo
		wantImport    string
		wantBase      string
		wantSignature string
	}{
		{
			name: "sub package model",
			info: &ModelInfo{
				ModulePath:   "helloworld",
				ModelPkgName: "group",
				ModelName:    "Group",
				ModelVarName: "g",
				ModelFileDir: "model/group",
				Design:       &dsl.Design{},
			},
			wantImport:    "\"github.com/hydroan/gst/model\"",
			wantBase:      "service.Base[*group.Group, *model.Empty, *group.GroupListRsp]",
			wantSignature: "func (g *Lister) List(ctx *types.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error)",
		},
		{
			name: "root model package",
			info: &ModelInfo{
				ModulePath:   "helloworld",
				ModelPkgName: "model",
				ModelName:    "Group",
				ModelVarName: "g",
				ModelFileDir: "model",
				Design:       &dsl.Design{},
			},
			wantImport:    "gstmodel \"github.com/hydroan/gst/model\"",
			wantBase:      "service.Base[*model.Group, *gstmodel.Empty, *model.GroupListRsp]",
			wantSignature: "func (g *Lister) List(ctx *types.ServiceContext, req *gstmodel.Empty) (rsp *model.GroupListRsp, err error)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := &dsl.Action{
				Enabled: true,
				Service: true,
				Payload: dsl.PayloadEmpty,
				Result:  "*" + tt.info.ModelName + "ListRsp",
				Phase:   consts.PHASE_LIST,
			}
			file := GenerateServiceWithPackage(tt.info, action, consts.PHASE_LIST, "group")
			if file == nil {
				t.Fatal("GenerateServiceWithPackage returned nil")
			}
			got, err := FormatNodeExtra(file)
			if err != nil {
				t.Fatalf("format generated service failed: %v", err)
			}
			for _, want := range []string{tt.wantImport, tt.wantBase, tt.wantSignature} {
				if !strings.Contains(got, want) {
					t.Errorf("generated service missing %q, got:\n%s", want, got)
				}
			}
		})
	}
}
