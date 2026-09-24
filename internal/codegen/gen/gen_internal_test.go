package gen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/kr/pretty"
)

var defaultImportModelSource = `
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

var namedImportModelSource = `
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

func TestFindModels(t *testing.T) {
	// The model files live in a directory of their own: a go.mod written into
	// the package directory would briefly turn it into a module of its own.
	dir := t.TempDir()
	t.Chdir(dir)
	modulePath := "github.com/hydroan/gst"
	if err := os.WriteFile("go.mod", []byte("module "+modulePath), 0o600); err != nil {
		t.Fatal(err)
	}

	modelDir := filepath.Join(dir, "model")
	if err := os.MkdirAll(modelDir, 0o750); err != nil {
		t.Fatal(err)
	}

	filename1 := filepath.Join(modelDir, "user.go")
	filename2 := filepath.Join(modelDir, "user2.go")
	if err := os.WriteFile(filename1, []byte(defaultImportModelSource), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename2, []byte(namedImportModelSource), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		modulePath string
		modelDir   string
		filename   string
		want       []*ModelInfo
	}{
		{
			name:       "default",
			modulePath: modulePath,
			modelDir:   modelDir,
			filename:   filename1,
			want: []*ModelInfo{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
		},
		{
			name:       "named",
			modulePath: modulePath,
			modelDir:   modelDir,
			filename:   filename2,
			want: []*ModelInfo{
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*User", Result: "*User"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*Group", Result: "*Group"},
					},
				},
				{
					ModulePath:    "github.com/hydroan/gst",
					ModelFileDir:  modelDir,
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
						SSE:        &dsl.Action{Payload: "*Device", Result: "*Device"},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FindModels(tt.modulePath, tt.modelDir, tt.filename)
			if err != nil {
				t.Fatalf("FindModels() failed: %v", err)
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

func TestHumanizeDSLFilename(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"archive_sample_items", "archive sample items"},
		{"archive-sample-items", "archive sample items"},
		{"path/to/foo_bar-baz", "foo bar baz"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := humanizeDSLFilename(tt.in); got != tt.want {
				t.Fatalf("humanizeDSLFilename(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestServiceActionLogQuoted(t *testing.T) {
	t.Parallel()
	act := &dsl.Action{Filename: "archive_sample_items"}
	if got := serviceActionLogQuoted("Record", consts.PHASE_CREATE, act); got != `"record: archive sample items"` {
		t.Fatalf("main create: got %s", got)
	}
	if got := serviceActionLogQuoted("Record", consts.PHASE_CREATE_BEFORE, act); got != `"record: archive sample items before"` {
		t.Fatalf("before hook: got %s", got)
	}
	if got := serviceActionLogQuoted("Record", consts.PHASE_CREATE_AFTER, act); got != `"record: archive sample items after"` {
		t.Fatalf("after hook: got %s", got)
	}
	act2 := &dsl.Action{Filename: "archive-sample-items"}
	if got := serviceActionLogQuoted("Record", consts.PHASE_CREATE, act2); got != `"record: archive sample items"` {
		t.Fatalf("hyphen filename: got %s", got)
	}
	if got := serviceActionLogQuoted("User", consts.PHASE_CREATE, nil); got != `"user create"` {
		t.Fatalf("no Filename: got %s", got)
	}
}

func TestGenServiceMethod1(t *testing.T) {
	tests := []struct {
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod1 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_CREATE_BEFORE,
			want: `func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")

	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod1(tt.info, tt.info.ModelPkgName, nil, tt.phase, tt.phase.RoleName()))
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
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod2 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_LIST_BEFORE,
			want: `func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list before")

	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod2(tt.info, tt.info.ModelPkgName, nil, tt.phase, tt.phase.RoleName()))
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
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod3 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_CREATE_MANY_BEFORE,
			want: `func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create many before")

	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod3(tt.info, tt.info.ModelPkgName, nil, tt.phase, tt.phase.RoleName()))
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
		name    string
		info    *ModelInfo
		reqName string
		rspName string
		phase   consts.Phase
		want    string
	}{
		{
			// The example of the genServiceMethod4 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "*User",
			rspName: "*User",
			phase:   consts.PHASE_CREATE,
			want: `func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")

	return rsp, nil
}`,
		},
		{
			// A bare action type name (the declared form of slice and map
			// action types) is transcribed as a value type.
			name: "group_bare_names_transcribed",
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
			want: `func (g *Updater) Update(ctx *gst.ServiceContext, req model.GroupRequest) (rsp model.GroupResponse, err error) {
	log := g.WithContext(ctx, ctx.Phase())
	log.Info("group update")

	return rsp, nil
}`,
		},
		{
			name: "group_starred_names_transcribed",
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
			want: `func (g *Updater) Update(ctx *gst.ServiceContext, req *model.GroupRequest) (rsp *model.GroupResponse, err error) {
	log := g.WithContext(ctx, ctx.Phase())
	log.Info("group update")

	return rsp, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := genServiceMethod4(tt.info, tt.info.ModelPkgName, nil, tt.reqName, tt.rspName, tt.phase, tt.phase.RoleName())
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
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod5 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_IMPORT,
			want: `func (u *Importer) Import(ctx *gst.ServiceContext, reader io.Reader) (users []*model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user import")

	return users, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod5(tt.info, tt.info.ModelPkgName, nil, tt.phase, tt.phase.RoleName()))
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
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod6 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.PHASE_EXPORT,
			want: `func (u *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user export")

	return data, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod6(tt.info, tt.info.ModelPkgName, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod6() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod7(t *testing.T) {
	tests := []struct {
		name  string
		info  *ModelInfo
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod7 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.PHASE_SSE,
			want: `func (u *Streamer) SSE(ctx *gst.ServiceContext) (err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user sse")

	return nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod7(tt.info, nil, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod7() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestGenServiceMethod8(t *testing.T) {
	tests := []struct {
		name   string
		info   *ModelInfo
		action *dsl.Action
		phase  consts.Phase
		role   string
		want   string
	}{
		{
			// The example of the genServiceMethod8 doc comment.
			name: "user",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.PHASE_LIST,
			role:  "Lister",
			want: `func (u *Lister) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list filter")

	return user, opts, nil
}`,
		},
		{
			// The Export action reuses the list pipeline, Filter included.
			name: "export",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.PHASE_EXPORT,
			role:  "Exporter",
			want: `func (u *Exporter) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user export filter")

	return user, opts, nil
}`,
		},
		{
			// A Filename action logs its label, like the other hooks do.
			name: "filename",
			info: &ModelInfo{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "s",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			action: &dsl.Action{Filename: "search"},
			phase:  consts.PHASE_LIST,
			role:   "Search",
			want: `func (s *Search) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
	log := s.WithContext(ctx, ctx.Phase())
	log.Info("user: search filter")

	return user, opts, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(genServiceMethod8(tt.info, tt.info.ModelPkgName, tt.action, tt.phase, tt.role))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("genServiceMethod8() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

// TestGenerateServiceCreate compares the whole service file GenerateService
// builds for the example of its doc comment: a Create action on a database
// model, which gets the before and after hooks.
func TestGenerateServiceCreate(t *testing.T) {
	info := &ModelInfo{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{},
	}
	action := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_CREATE,
	}

	file := GenerateService(info, action, consts.PHASE_CREATE, "user")
	if file == nil {
		t.Fatal("GenerateService returned nil")
	}
	got, err := FormatNodeExtra(file)
	if err != nil {
		t.Fatalf("format generated service failed: %v", err)
	}
	want := `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")

	return rsp, nil
}

func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")

	return nil
}

func (u *Creator) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create after")

	return nil
}
`
	if got != want {
		t.Errorf("GenerateService() =\n%s\nwant\n%s", got, want)
	}
}

// TestGenerateServiceList compares the whole service file GenerateService
// builds for a List action on a database model: the List method, then the
// hooks in the order the list controller invokes them, ListBefore, Filter
// and ListAfter.
func TestGenerateServiceList(t *testing.T) {
	info := &ModelInfo{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{},
	}
	action := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_LIST,
	}

	file := GenerateService(info, action, consts.PHASE_LIST, "user")
	if file == nil {
		t.Fatal("GenerateService returned nil")
	}
	got, err := FormatNodeExtra(file)
	if err != nil {
		t.Fatalf("format generated service failed: %v", err)
	}
	want := `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *Lister) List(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list")

	return rsp, nil
}

func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list before")

	return nil
}

func (u *Lister) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list filter")

	return user, opts, nil
}

func (u *Lister) ListAfter(ctx *gst.ServiceContext, users *[]*model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user list after")

	return nil
}
`
	if got != want {
		t.Errorf("GenerateService() =\n%s\nwant\n%s", got, want)
	}
}

func TestGenerateServiceListEmptyPayload(t *testing.T) {
	tests := []struct {
		name          string
		info          *ModelInfo
		wantImport    string
		wantBase      string
		wantSignature string
		wantFilter    string
	}{
		{
			name: "sub_package_model",
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
			wantSignature: "func (g *Lister) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error)",
			wantFilter:    "func (g *Lister) Filter(ctx *gst.ServiceContext, group *group.Group, opts gst.QueryOptions) (*group.Group, gst.QueryOptions, error)",
		},
		{
			name: "root_model_package",
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
			wantSignature: "func (g *Lister) List(ctx *gst.ServiceContext, req *gstmodel.Empty) (rsp *model.GroupListRsp, err error)",
			wantFilter:    "func (g *Lister) Filter(ctx *gst.ServiceContext, group *model.Group, opts gst.QueryOptions) (*model.Group, gst.QueryOptions, error)",
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
			file := GenerateService(tt.info, action, consts.PHASE_LIST, "group")
			if file == nil {
				t.Fatal("GenerateService returned nil")
			}
			got, err := FormatNodeExtra(file)
			if err != nil {
				t.Fatalf("format generated service failed: %v", err)
			}
			for _, want := range []string{tt.wantImport, tt.wantBase, tt.wantSignature, tt.wantFilter} {
				if !strings.Contains(got, want) {
					t.Errorf("generated service missing %q, got:\n%s", want, got)
				}
			}
		})
	}
}

func TestGenerateServiceExport(t *testing.T) {
	newInfo := func(isEmpty bool) *ModelInfo {
		return &ModelInfo{
			ModulePath:   "helloworld",
			ModelPkgName: "model",
			ModelName:    "User",
			ModelVarName: "u",
			ModelFileDir: "model",
			Design:       &dsl.Design{IsEmpty: isEmpty},
		}
	}
	// dsl.Parse defaults an action's Payload/Result to the starred model name.
	action := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_EXPORT,
	}

	// The export controller invocation order: ListBefore, Filter, ListAfter,
	// Export.
	hookSigsInOrder := []string{
		"func (u *Exporter) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error",
		"func (u *Exporter) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error)",
		"func (u *Exporter) ListAfter(ctx *gst.ServiceContext, users *[]*model.User) error",
	}
	exportSig := "func (u *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error)"

	t.Run("non-empty_model_generates_list_hooks_in_controller_invocation_order", func(t *testing.T) {
		file := GenerateService(newInfo(false), action, consts.PHASE_EXPORT, "user")
		if file == nil {
			t.Fatal("GenerateService returned nil")
		}
		got, err := FormatNodeExtra(file)
		if err != nil {
			t.Fatalf("format generated service failed: %v", err)
		}
		lastIdx := -1
		for _, sig := range append(append([]string{}, hookSigsInOrder...), exportSig) {
			idx := strings.Index(got, sig)
			if idx < 0 {
				t.Fatalf("generated service missing %q, got:\n%s", sig, got)
			}
			if idx <= lastIdx {
				t.Fatalf("generated method %q out of controller invocation order, got:\n%s", sig, got)
			}
			lastIdx = idx
		}
	})

	t.Run("empty_model_generates_Export_only", func(t *testing.T) {
		file := GenerateService(newInfo(true), action, consts.PHASE_EXPORT, "user")
		if file == nil {
			t.Fatal("GenerateService returned nil")
		}
		got, err := FormatNodeExtra(file)
		if err != nil {
			t.Fatalf("format generated service failed: %v", err)
		}
		if !strings.Contains(got, exportSig) {
			t.Errorf("generated service missing %q, got:\n%s", exportSig, got)
		}
		for _, sig := range hookSigsInOrder {
			if strings.Contains(got, sig) {
				t.Errorf("generated service for empty model must not contain %q, got:\n%s", sig, got)
			}
		}
	})
}

func TestGenerateServiceSSE(t *testing.T) {
	info := &ModelInfo{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{IsEmpty: true},
	}
	// dsl.Parse defaults an action's Payload/Result to the starred model name.
	action := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_SSE,
	}

	file := GenerateService(info, action, consts.PHASE_SSE, "user")
	if file == nil {
		t.Fatal("GenerateService returned nil")
	}
	got, err := FormatNodeExtra(file)
	if err != nil {
		t.Fatalf("format generated service failed: %v", err)
	}

	structDecl := "type Streamer struct"
	if !strings.Contains(got, structDecl) {
		t.Errorf("generated service missing %q, got:\n%s", structDecl, got)
	}
	sseSig := "func (u *Streamer) SSE(ctx *gst.ServiceContext) (err error)"
	if !strings.Contains(got, sseSig) {
		t.Errorf("generated service missing %q, got:\n%s", sseSig, got)
	}
	for _, hook := range []string{"Before", "After"} {
		if strings.Contains(got, hook) {
			t.Errorf("generated SSE service must not scaffold %s hooks, got:\n%s", hook, got)
		}
	}
}
