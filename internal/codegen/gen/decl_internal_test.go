package gen

import (
	"bytes"
	"go/format"
	"go/token"
	"reflect"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/kr/pretty"
)

func TestImports(t *testing.T) {
	tests := []struct {
		name           string
		modulePath     string
		modelFileDir   string
		modelQualifier string
		phase          consts.Phase
		otherPkgs      []string
		want           string
	}{
		{
			name:           "root_model_package",
			modulePath:     "codegen",
			modelFileDir:   "model",
			modelQualifier: "model",
			phase:          consts.PHASE_CREATE,
			want: `import (
	"codegen/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
)`,
		},
		{
			name:           "other_package",
			modulePath:     "codegen",
			modelFileDir:   "model/group",
			modelQualifier: "group",
			phase:          consts.PHASE_CREATE,
			otherPkgs:      []string{"github.com/hydroan/gst/model"},
			want: `import (
	"codegen/model/group"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
)`,
		},
		{
			name:           "aliased_other_package",
			modulePath:     "codegen",
			modelFileDir:   "model",
			modelQualifier: "model",
			phase:          consts.PHASE_CREATE,
			otherPkgs:      []string{"gstmodel github.com/hydroan/gst/model"},
			want: `import (
	"codegen/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
	gstmodel "github.com/hydroan/gst/model"
)`,
		},
		{
			// The package in model/record_item is named recorditem, which the
			// import states.
			name:           "package_named_unlike_its_directory",
			modulePath:     "codegen",
			modelFileDir:   "model/record_item",
			modelQualifier: "recorditem",
			phase:          consts.PHASE_CREATE,
			want: `import (
	recorditem "codegen/model/record_item"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
)`,
		},
		{
			// A model package named service takes an alias in the file, which
			// imports the gst service package as service.
			name:           "aliased_model_package",
			modulePath:     "codegen",
			modelFileDir:   "model/service",
			modelQualifier: "model_service",
			phase:          consts.PHASE_CREATE,
			want: `import (
	model_service "codegen/model/service"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
)`,
		},
		{
			// The example of the imports doc comment: an Import action reads
			// through io, which the model package in model/io takes an alias
			// for.
			name:           "import_action_reads_through_io",
			modulePath:     "helloworld",
			modelFileDir:   "model/io",
			modelQualifier: "model_io",
			phase:          consts.PHASE_IMPORT,
			want: `import (
	model_io "helloworld/model/io"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst"
	"io"
)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(imports(tt.modulePath, tt.modelFileDir, tt.modelQualifier, tt.phase, tt.otherPkgs...))
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("imports() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceModelQualifier(t *testing.T) {
	tests := []struct {
		name         string
		modelFileDir string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "package_name_no_framework_import_takes",
			modelFileDir: "model/sample",
			modelPkgName: "sample",
			phase:        consts.PHASE_CREATE,
			want:         "sample",
		},
		{
			// The gst model package a model.Empty request needs is the one
			// that yields its name, as gstmodel (see emptyReqPkgName).
			name:         "root_model_package",
			modelFileDir: "model",
			modelPkgName: "model",
			phase:        consts.PHASE_LIST,
			want:         "model",
		},
		{
			// The example of the serviceModelQualifier doc comment.
			name:         "package_named_service",
			modelFileDir: "model/service",
			modelPkgName: "service",
			phase:        consts.PHASE_CREATE,
			want:         "model_service",
		},
		{
			name:         "nested_package_named_service",
			modelFileDir: "model/sample/service",
			modelPkgName: "service",
			phase:        consts.PHASE_CREATE,
			want:         "sample_service",
		},
		{
			name:         "package_named_gst",
			modelFileDir: "model/gst",
			modelPkgName: "gst",
			phase:        consts.PHASE_GET,
			want:         "model_gst",
		},
		{
			name:         "package_named_io_in_an_import_action",
			modelFileDir: "model/io",
			modelPkgName: "io",
			phase:        consts.PHASE_IMPORT,
			want:         "model_io",
		},
		{
			// Only the service file of an Import action imports io.
			name:         "package_named_io_in_any_other_action",
			modelFileDir: "model/io",
			modelPkgName: "io",
			phase:        consts.PHASE_CREATE,
			want:         "io",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &ModelInfo{ModulePath: "helloworld", ModelFileDir: tt.modelFileDir, ModelPkgName: tt.modelPkgName}
			if got := serviceModelQualifier(info, tt.phase); got != tt.want {
				t.Errorf("serviceModelQualifier() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTypes(t *testing.T) {
	tests := []struct {
		name         string
		modelPkgName string
		modelName    string
		reqName      string
		rspName      string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "user",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      "*User",
			rspName:      "*User",
			phase:        consts.PHASE_CREATE,
			want: `type Creator struct {
	service.Base[*model.User, *model.User, *model.User]
}`,
		},
		{
			// The second example of the types doc comment.
			name:         "model_package_under_an_alias",
			modelPkgName: "model_service",
			modelName:    "User",
			reqName:      "*UserReq",
			rspName:      "*UserRsp",
			phase:        consts.PHASE_UPDATE,
			want: `type Updater struct {
	service.Base[*model_service.User, *model_service.UserReq, *model_service.UserRsp]
}`,
		},
		{
			// Bare action type names (the declared form of slice and map
			// action types) are transcribed as value types.
			name:         "user_bare_names_transcribed",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      "UserReq",
			rspName:      "UserRsp",
			phase:        consts.PHASE_UPDATE,
			want: `type Updater struct {
	service.Base[*model.User, model.UserReq, model.UserRsp]
}`,
		},
		{
			// The first example of the types doc comment.
			name:         "user_starred_names_transcribed",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      "*UserReq",
			rspName:      "*UserRsp",
			phase:        consts.PHASE_UPDATE,
			want: `type Updater struct {
	service.Base[*model.User, *model.UserReq, *model.UserRsp]
}`,
		},
		{
			name:         "list_with_empty_payload",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			phase:        consts.PHASE_LIST,
			want: `type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}`,
		},
		{
			name:         "list_with_empty_payload_in_root_model_package",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*UserListRsp",
			phase:        consts.PHASE_LIST,
			want: `type Lister struct {
	service.Base[*model.User, *gstmodel.Empty, *model.UserListRsp]
}`,
		},
		{
			name:         "create_with_empty_result",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      "*GroupCreateReq",
			rspName:      dsl.PayloadEmpty,
			phase:        consts.PHASE_CREATE,
			want: `type Creator struct {
	service.Base[*group.Group, *group.GroupCreateReq, *model.Empty]
}`,
		},
		{
			name:         "create_with_empty_result_in_root_model_package",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      "*UserCreateReq",
			rspName:      dsl.PayloadEmpty,
			phase:        consts.PHASE_CREATE,
			want: `type Creator struct {
	service.Base[*model.User, *model.UserCreateReq, *gstmodel.Empty]
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := types(tt.modelPkgName, tt.modelName, tt.reqName, tt.rspName, tt.phase.RoleName())
			var buf bytes.Buffer
			fset := token.NewFileSet()
			if err := format.Node(&buf, fset, res); err != nil {
				t.Error(err)
				return
			}
			got := buf.String()
			if got != tt.want {
				t.Errorf("types() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod1(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			// The examples of the serviceMethod1 doc comment, with UpdateAfter.
			name:         "CreateBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_CREATE_BEFORE,
			want:         "func (u *Creator) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {\n}",
		},
		{
			name:         "UpdateAfter",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_UPDATE_AFTER,
			want:         "func (g *Updater) UpdateAfter(ctx *gst.ServiceContext, group *model_auth.Group) error {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(serviceMethod1(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName()))
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod1() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod2(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			// The example of the serviceMethod2 doc comment.
			name:         "ListBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_LIST_BEFORE,
			want:         "func (u *Lister) ListBefore(ctx *gst.ServiceContext, users *[]*model.User) error {\n}",
		},
		{
			name:         "ListAfter",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_LIST_AFTER,
			want:         "func (g *Lister) ListAfter(ctx *gst.ServiceContext, groups *[]*model_auth.Group) error {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod2(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod2() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod3(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			// The example of the serviceMethod3 doc comment.
			name:         "CreateManyBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_CREATE_MANY_BEFORE,
			want:         "func (u *ManyCreator) CreateManyBefore(ctx *gst.ServiceContext, users ...*model.User) error {\n}",
		},
		{
			name:         "UpdateManyBefore",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_UPDATE_MANY_BEFORE,
			want:         "func (g *ManyUpdater) UpdateManyBefore(ctx *gst.ServiceContext, groups ...*model_auth.Group) error {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod3(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod3() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod4(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelPkgName string
		reqName      string
		rspName      string
		phase        consts.Phase
		want         string
	}{
		{
			// The example of the serviceMethod4 doc comment.
			name:         "Create",
			recvName:     "u",
			modelPkgName: "model",
			reqName:      "*User",
			rspName:      "*User",
			phase:        consts.PHASE_CREATE,
			want:         "func (u *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {\n}",
		},
		{
			// Bare action type names (the declared form of slice and map
			// action types) are transcribed as value types.
			name:         "UpdateBareNamesTranscribed",
			recvName:     "g",
			modelPkgName: "model",
			reqName:      "GroupRequest",
			rspName:      "GroupResponse",
			phase:        consts.PHASE_UPDATE,
			want:         "func (g *Updater) Update(ctx *gst.ServiceContext, req model.GroupRequest) (rsp model.GroupResponse, err error) {\n}",
		},
		{
			name:         "UpdateStarredNamesTranscribed",
			recvName:     "g",
			modelPkgName: "model",
			reqName:      "*GroupRequest",
			rspName:      "*GroupResponse",
			phase:        consts.PHASE_UPDATE,
			want:         "func (g *Updater) Update(ctx *gst.ServiceContext, req *model.GroupRequest) (rsp *model.GroupResponse, err error) {\n}",
		},
		{
			name:         "ListEmptyPayload",
			recvName:     "g",
			modelPkgName: "group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			phase:        consts.PHASE_LIST,
			want:         "func (g *Lister) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {\n}",
		},
		{
			name:         "GetEmptyPayloadRootModelPackage",
			recvName:     "u",
			modelPkgName: "model",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*UserGetRsp",
			phase:        consts.PHASE_GET,
			want:         "func (u *Getter) Get(ctx *gst.ServiceContext, req *gstmodel.Empty) (rsp *model.UserGetRsp, err error) {\n}",
		},
		{
			name:         "CreateEmptyResult",
			recvName:     "g",
			modelPkgName: "group",
			reqName:      "*GroupCreateReq",
			rspName:      dsl.PayloadEmpty,
			phase:        consts.PHASE_CREATE,
			want:         "func (g *Creator) Create(ctx *gst.ServiceContext, req *group.GroupCreateReq) (rsp *model.Empty, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod4(tt.recvName, tt.modelPkgName, tt.reqName, tt.rspName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}

			if got != tt.want {
				t.Errorf("serviceMethod4() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod5(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			// The example of the serviceMethod5 doc comment.
			name:         "Import",
			recvName:     "a",
			modelName:    "Sample",
			modelPkgName: "model",
			phase:        consts.PHASE_IMPORT,
			want:         "func (a *Importer) Import(ctx *gst.ServiceContext, reader io.Reader) (samples []*model.Sample, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod5(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}

			if got != tt.want {
				t.Errorf("serviceMethod5() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod6(t *testing.T) {
	tests := []struct {
		name         string
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			// The example of the serviceMethod6 doc comment.
			name:         "Export",
			recvName:     "a",
			modelName:    "Sample",
			modelPkgName: "model",
			phase:        consts.PHASE_EXPORT,
			want:         "func (a *Exporter) Export(ctx *gst.ServiceContext, samples ...*model.Sample) (data []byte, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod6(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod6() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod7(t *testing.T) {
	tests := []struct {
		name     string
		recvName string
		phase    consts.Phase
		want     string
	}{
		{
			// The example of the serviceMethod7 doc comment.
			name:     "SSE",
			recvName: "a",
			phase:    consts.PHASE_SSE,
			want:     "func (a *Streamer) SSE(ctx *gst.ServiceContext) (err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod7(tt.recvName, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod7() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestServiceMethod8(t *testing.T) {
	tests := []struct {
		name           string
		recvName       string
		modelName      string
		modelQualifier string
		roleName       string
		want           string
	}{
		{
			// The example of the serviceMethod8 doc comment.
			name:           "Filter",
			recvName:       "u",
			modelName:      "User",
			modelQualifier: "model",
			roleName:       "Lister",
			want:           "func (u *Lister) Filter(ctx *gst.ServiceContext, user *model.User, opts gst.QueryOptions) (*model.User, gst.QueryOptions, error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod8(tt.recvName, tt.modelName, tt.modelQualifier, tt.roleName)
			got, err := FormatNode(res)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod8() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}
