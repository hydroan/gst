package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"reflect"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
	"github.com/kr/pretty"
)

func TestImports(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		modulePath   string
		modelFileDir string
		modelPkgName string
		otherPkgs    []string

		want string
	}{
		{
			modulePath:   "codegen",
			modelFileDir: "model",
			modelPkgName: "model",
			want: `import (
	"codegen/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)`,
		},
		{
			name:         "other package",
			modulePath:   "codegen",
			modelFileDir: "model/group",
			modelPkgName: "group",
			otherPkgs:    []string{"github.com/hydroan/gst/model"},
			want: `import (
	"codegen/model/group"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/model"
)`,
		},
		{
			name:         "aliased other package",
			modulePath:   "codegen",
			modelFileDir: "model",
			modelPkgName: "model",
			otherPkgs:    []string{"gstmodel github.com/hydroan/gst/model"},
			want: `import (
	"codegen/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	gstmodel "github.com/hydroan/gst/model"
)`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(imports(tt.modulePath, tt.modelFileDir, tt.modelPkgName, tt.otherPkgs...))
			if err != nil {
				t.Error(err)
				return
			}
			fmt.Println(got)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("imports() = \n%v\n, want \n%v\n", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestInits(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		modelName string
		want      string
	}{
		{
			name:      "user",
			modelName: "User",
			want: `func init() {
	service.Register[*user]()
}`,
		},
		{
			name:      "group",
			modelName: "Group",
			want: `func init() {
	service.Register[*group]()
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FormatNode(inits(tt.modelName))
			if err != nil {
				t.Error(err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("inits() = \n%v\n, want \n%v\n", fmt.Sprintf("% #v", got), fmt.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestTypes(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		modelPkgname string
		modelName    string
		reqName      string
		rspName      string
		phase        consts.Phase
		withComments bool
		want         string
	}{
		{
			name:         "user",
			modelPkgname: "model",
			modelName:    "User",
			reqName:      "User",
			rspName:      "User",
			phase:        consts.PHASE_CREATE,
			want: `type Creator struct {
	service.Base[*model.User, *model.User, *model.User]
}`,
		},
		{
			name:         "user",
			modelPkgname: "model",
			modelName:    "User",
			reqName:      "UserReq",
			rspName:      "UserRsp",
			phase:        consts.PHASE_UPDATE,
			want: `type Updater struct {
	service.Base[*model.User, model.UserReq, model.UserRsp]
}`,
		},
		{
			name:         "user2",
			modelPkgname: "model",
			modelName:    "User",
			reqName:      "*UserReq",
			rspName:      "*UserRsp",
			phase:        consts.PHASE_UPDATE,
			want: `type Updater struct {
	service.Base[*model.User, *model.UserReq, *model.UserRsp]
}`,
		},
		{
			name:         "list with empty payload",
			modelPkgname: "group",
			modelName:    "Group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			phase:        consts.PHASE_LIST,
			want: `type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}`,
		},
		{
			name:         "list with empty payload in root model package",
			modelPkgname: "model",
			modelName:    "User",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*UserListRsp",
			phase:        consts.PHASE_LIST,
			want: `type Lister struct {
	service.Base[*model.User, *gstmodel.Empty, *model.UserListRsp]
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := types(tt.modelPkgname, tt.modelName, tt.reqName, tt.rspName, tt.phase, tt.phase.RoleName(), tt.withComments)
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "CreateBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_CREATE_BEFORE,
			want:         "func (u *Creator) CreateBefore(ctx *types.ServiceContext, user *model.User) error {\n}",
		},
		{
			name:         "UpdateAfter",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_UPDATE_AFTER,
			want:         "func (g *Updater) UpdateAfter(ctx *types.ServiceContext, group *model_auth.Group) error {\n}",
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "ListBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_LIST_BEFORE,
			want:         "func (u *Lister) ListBefore(ctx *types.ServiceContext, users *[]*model.User) error {\n}",
		},
		{
			name:         "ListAfter",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_LIST_AFTER,
			want:         "func (g *Lister) ListAfter(ctx *types.ServiceContext, groups *[]*model_auth.Group) error {\n}",
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		phase        consts.Phase
		modelPkgName string
		want         string
	}{
		{
			name:         "CreateManyBefore",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_CREATE_MANY_BEFORE,
			want:         "func (u *ManyCreator) CreateManyBefore(ctx *types.ServiceContext, users ...*model.User) error {\n}",
		},
		{
			name:         "UpdateManyBefore",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_UPDATE_MANY_BEFORE,
			want:         "func (g *ManyUpdater) UpdateManyBefore(ctx *types.ServiceContext, groups ...*model_auth.Group) error {\n}",
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		reqName      string
		rspName      string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "Create",
			recvName:     "u",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      "User",
			rspName:      "User",
			phase:        consts.PHASE_CREATE,
			want:         "func (u *Creator) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {\n}",
		},
		{
			name:         "Update",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model",
			reqName:      "GroupRequest",
			rspName:      "GroupResponse",
			phase:        consts.PHASE_UPDATE,
			want:         "func (g *Updater) Update(ctx *types.ServiceContext, req model.GroupRequest) (rsp model.GroupResponse, err error) {\n}",
		},
		{
			name:         "Update2",
			recvName:     "g",
			modelPkgName: "model",
			modelName:    "Group",
			reqName:      "*GroupRequest",
			rspName:      "*GroupResponse",
			phase:        consts.PHASE_UPDATE,
			want:         "func (g *Updater) Update(ctx *types.ServiceContext, req *model.GroupRequest) (rsp *model.GroupResponse, err error) {\n}",
		},
		{
			name:         "ListEmptyPayload",
			recvName:     "g",
			modelPkgName: "group",
			modelName:    "Group",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*GroupListRsp",
			phase:        consts.PHASE_LIST,
			want:         "func (g *Lister) List(ctx *types.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {\n}",
		},
		{
			name:         "GetEmptyPayloadRootModelPackage",
			recvName:     "u",
			modelPkgName: "model",
			modelName:    "User",
			reqName:      dsl.PayloadEmpty,
			rspName:      "*UserGetRsp",
			phase:        consts.PHASE_GET,
			want:         "func (u *Getter) Get(ctx *types.ServiceContext, req *gstmodel.Empty) (rsp *model.UserGetRsp, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod4(tt.recvName, tt.modelName, tt.modelPkgName, tt.reqName, tt.rspName, tt.phase, tt.phase.RoleName())
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "dns",
			recvName:     "a",
			modelName:    "Asset",
			modelPkgName: "model",
			phase:        consts.PHASE_IMPORT,
			want:         "func (a *Importer) Import(ctx *types.ServiceContext, reader io.Reader) (assets []*model.Asset, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod5(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			recvName:     "a",
			modelName:    "Asset",
			modelPkgName: "model",
			phase:        consts.PHASE_EXPORT,
			want:         "func (a *Exporter) Export(ctx *types.ServiceContext, assets ...*model.Asset) (data []byte, err error) {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod6(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
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
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "Filter",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_FILTER,
			want:         "func (u *Lister) Filter(ctx *types.ServiceContext, user *model.User) *model.User {\n}",
		},
		{
			name:         "Filter2",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_FILTER,
			want:         "func (g *Lister) Filter(ctx *types.ServiceContext, group *model_auth.Group) *model_auth.Group {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := serviceMethod7(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(res)
			if err != nil {
				t.Fatal(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod7() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestServiceMethod8(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		recvName     string
		modelName    string
		modelPkgName string
		phase        consts.Phase
		want         string
	}{
		{
			name:         "FilterRaw",
			recvName:     "u",
			modelName:    "User",
			modelPkgName: "model",
			phase:        consts.PHASE_FILTER_RAW,
			want:         "func (u *Lister) FilterRaw(ctx *types.ServiceContext) string {\n}",
		},
		{
			name:         "FilterRaw2",
			recvName:     "g",
			modelName:    "Group",
			modelPkgName: "model_auth",
			phase:        consts.PHASE_FILTER_RAW,
			want:         "func (g *Lister) FilterRaw(ctx *types.ServiceContext) string {\n}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := serviceMethod8(tt.recvName, tt.modelName, tt.modelPkgName, tt.phase, tt.phase.RoleName())
			got, err := FormatNode(node)
			if err != nil {
				t.Fatal(err)
				return
			}
			if got != tt.want {
				t.Errorf("serviceMethod8() = %v, want %v", got, tt.want)
			}
		})
	}
}
