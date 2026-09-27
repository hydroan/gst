package gggen

import (
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/internal/dsl"
	"github.com/hydroan/gst/internal/modelinfo"
	"github.com/kr/pretty"
)

func TestServiceActionLogQuoted(t *testing.T) {
	t.Parallel()
	act := &dsl.Action{ServiceName: "archive_sample_items"}
	if got := serviceActionLogQuoted("Record", consts.Create, act); got != `"record: archive sample items"` {
		t.Fatalf("main create: got %s", got)
	}
	if got := serviceActionLogQuoted("Record", consts.CreateBefore, act); got != `"record: archive sample items before"` {
		t.Fatalf("before hook: got %s", got)
	}
	if got := serviceActionLogQuoted("Record", consts.CreateAfter, act); got != `"record: archive sample items after"` {
		t.Fatalf("after hook: got %s", got)
	}
	if got := serviceActionLogQuoted("User", consts.Create, nil); got != `"user create"` {
		t.Fatalf("no ServiceName: got %s", got)
	}
}

func TestGenServiceMethod1(t *testing.T) {
	tests := []struct {
		name  string
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod1 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.CreateBefore,
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
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod2 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.ListBefore,
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
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod3 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.CreateManyBefore,
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
		info    *modelinfo.Model
		reqName string
		rspName string
		phase   consts.Phase
		want    string
	}{
		{
			// The example of the genServiceMethod4 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "*User",
			rspName: "*User",
			phase:   consts.Create,
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
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "Group",
				ModelVarName: "g",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "GroupRequest",
			rspName: "GroupResponse",
			phase:   consts.Update,
			want: `func (g *Updater) Update(ctx *gst.ServiceContext, req model.GroupRequest) (rsp model.GroupResponse, err error) {
	log := g.WithContext(ctx, ctx.Phase())
	log.Info("group update")

	return rsp, nil
}`,
		},
		{
			name: "group_starred_names_transcribed",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "Group",
				ModelVarName: "g",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			reqName: "*GroupRequest",
			rspName: "*GroupResponse",
			phase:   consts.Update,
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
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod5 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.Import,
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
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod6 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "/tmp/model",
			},
			phase: consts.Export,
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
		info  *modelinfo.Model
		phase consts.Phase
		want  string
	}{
		{
			// The example of the genServiceMethod7 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.SSE,
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
		info   *modelinfo.Model
		action *dsl.Action
		phase  consts.Phase
		role   string
		want   string
	}{
		{
			// The example of the genServiceMethod8 doc comment.
			name: "user",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.List,
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
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "u",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			phase: consts.Export,
			role:  "Exporter",
			want: `func (u *Exporter) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user export filter")

	return user, opts, nil
}`,
		},
		{
			// An action naming its service logs its label, like the other hooks do.
			name: "named_service",
			info: &modelinfo.Model{
				ModelPkgName: "model",
				ModelName:    "User",
				ModelVarName: "s",
				ModulePath:   "codegen",
				ModelFileDir: "model",
			},
			action: &dsl.Action{ServiceName: "search"},
			phase:  consts.List,
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

// TestGenServiceMethod9 pins the examples of the genServiceMethod9 doc
// comment: the Stream scaffold of a server stream and of a client stream.
func TestGenServiceMethod9(t *testing.T) {
	feed := &modelinfo.Model{ModelPkgName: "model", ModelName: "Feed", ModulePath: "codegen", ModelFileDir: "model"}
	tests := []struct {
		name   string
		action *dsl.Action
		want   string
	}{
		{
			name:   "watch",
			action: &dsl.Action{Service: true, ServiceName: "watch", Payload: "*FeedWatchReq", Result: "*FeedEvent", StreamingResult: true, Phase: consts.Stream},
			want: `func (w *Watch) Stream(ctx *gst.ServiceContext, req *model.FeedWatchReq, stream *grpc.ServerStream[*model.FeedEvent]) (err error) {
	log := w.WithContext(ctx, ctx.Phase())
	log.Info("feed: watch")

	return nil
}`,
		},
		{
			name:   "upload",
			action: &dsl.Action{Service: true, ServiceName: "upload", Payload: "*FeedEvent", Result: "*FeedUploadRsp", StreamingPayload: true, Phase: consts.Stream},
			want: `func (u *Upload) Stream(ctx *gst.ServiceContext, stream *grpc.ClientStream[*model.FeedEvent]) (rsp *model.FeedUploadRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("feed: upload")

	return rsp, nil
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := *feed
			info.ModelVarName = strings.ToLower(tt.action.RoleName()[:1])
			got, err := FormatNode(genServiceMethod9(&info, "model", tt.action, consts.Stream, tt.action.RoleName()))
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("genServiceMethod9() = \n%v\n, want \n%v\n", got, tt.want)
			}
		})
	}
}

// TestGenerateServiceCreate compares the whole service file GenerateService
// builds for the example of its doc comment: a Create action on a database
// model, which gets the before and after hooks.
func TestGenerateServiceCreate(t *testing.T) {
	info := &modelinfo.Model{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{},
	}
	action := &dsl.Action{
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.Create,
	}

	file := GenerateService(info, action, consts.Create, "user")
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
	info := &modelinfo.Model{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{},
	}
	action := &dsl.Action{
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.List,
	}

	file := GenerateService(info, action, consts.List, "user")
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
		info          *modelinfo.Model
		wantImport    string
		wantBase      string
		wantSignature string
		wantFilter    string
	}{
		{
			name: "sub_package_model",
			info: &modelinfo.Model{
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
			info: &modelinfo.Model{
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
				Service: true,
				Payload: dsl.PayloadEmpty,
				Result:  "*" + tt.info.ModelName + "ListRsp",
				Phase:   consts.List,
			}
			file := GenerateService(tt.info, action, consts.List, "group")
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
	newInfo := func(isEmpty bool) *modelinfo.Model {
		return &modelinfo.Model{
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
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.Export,
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
		file := GenerateService(newInfo(false), action, consts.Export, "user")
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
		file := GenerateService(newInfo(true), action, consts.Export, "user")
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
	info := &modelinfo.Model{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "User",
		ModelVarName: "u",
		ModelFileDir: "model",
		Design:       &dsl.Design{IsEmpty: true},
	}
	// dsl.Parse defaults an action's Payload/Result to the starred model name.
	action := &dsl.Action{
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.SSE,
	}

	file := GenerateService(info, action, consts.SSE, "user")
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

// TestGenerateServiceStream pins the service file of a Stream action: the
// struct embedding service.Base on the action's types, the Stream method of
// its kind, the gst grpc import the stream comes from, and no hook.
func TestGenerateServiceStream(t *testing.T) {
	info := &modelinfo.Model{
		ModulePath:   "helloworld",
		ModelPkgName: "model",
		ModelName:    "Feed",
		ModelVarName: "f",
		ModelFileDir: "model",
		Design:       &dsl.Design{},
	}
	action := &dsl.Action{Service: true, ServiceName: "chat", Payload: "*FeedEvent", Result: "*FeedEvent", StreamingPayload: true, StreamingResult: true, Phase: consts.Stream}

	file := GenerateService(info, action, consts.Stream, "feed")
	if file == nil {
		t.Fatal("GenerateService returned nil")
	}
	got, err := FormatNodeExtra(file)
	if err != nil {
		t.Fatalf("format generated service failed: %v", err)
	}

	for _, want := range []string{
		`"github.com/hydroan/gst/grpc"`,
		"type Chat struct",
		"service.Base[*model.Feed, *model.FeedEvent, *model.FeedEvent]",
		"func (c *Chat) Stream(ctx *gst.ServiceContext, stream *grpc.BidiStream[*model.FeedEvent, *model.FeedEvent]) (err error)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated service missing %q, got:\n%s", want, got)
		}
	}
	for _, hook := range []string{"Before", "After"} {
		if strings.Contains(got, hook) {
			t.Errorf("generated Stream service must not scaffold %s hooks, got:\n%s", hook, got)
		}
	}
}
