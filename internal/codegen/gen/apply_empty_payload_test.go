package gen

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/types/consts"
)

func TestApplyServiceFileEmptyPayload(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		action         *dsl.Action
		servicePkgName string
		wantContains   []string
		wantMissing    []string
	}{
		{
			name: "switch business req to empty payload adds gst model import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*group.Group, *group.GroupListReq, *group.GroupListRsp]
}

func (g *Lister) List(ctx *types.ServiceContext, req *group.GroupListReq) (rsp *group.GroupListRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Service: true,
				Payload: dsl.PayloadEmpty,
				Result:  "*GroupListRsp",
				Phase:   consts.PHASE_LIST,
			},
			servicePkgName: "group",
			wantContains: []string{
				`"github.com/hydroan/gst/model"`,
				"service.Base[*group.Group, *model.Empty, *group.GroupListRsp]",
				"req *model.Empty",
			},
		},
		{
			name: "switch empty payload back to model removes gst model import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}

func (g *Lister) List(ctx *types.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Service: true,
				Payload: "*Group",
				Result:  "*Group",
				Phase:   consts.PHASE_LIST,
			},
			servicePkgName: "group",
			wantContains: []string{
				"service.Base[*group.Group, *group.Group, *group.Group]",
				"req *group.Group",
			},
			wantMissing: []string{
				`"github.com/hydroan/gst/model"`,
			},
		},
		{
			name: "switch to empty payload in root model package uses gstmodel alias",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Getter struct {
	service.Base[*model.User, *model.UserGetReq, *model.UserGetRsp]
}

func (u *Getter) Get(ctx *types.ServiceContext, req *model.UserGetReq) (rsp *model.UserGetRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Service: true,
				Payload: dsl.PayloadEmpty,
				Result:  "*UserGetRsp",
				Phase:   consts.PHASE_GET,
			},
			servicePkgName: "user",
			wantContains: []string{
				`gstmodel "github.com/hydroan/gst/model"`,
				"service.Base[*model.User, *gstmodel.Empty, *model.UserGetRsp]",
				"req *gstmodel.Empty",
			},
		},
		{
			name: "empty payload apply is idempotent for existing import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}

func (g *Lister) List(ctx *types.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Service: true,
				Payload: dsl.PayloadEmpty,
				Result:  "*GroupListRsp",
				Phase:   consts.PHASE_LIST,
			},
			servicePkgName: "group",
			wantContains: []string{
				"service.Base[*group.Group, *model.Empty, *group.GroupListRsp]",
				"req *model.Empty",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}

			ApplyServiceFile(file, tt.action, tt.servicePkgName)

			got, err := FormatNodeExtraWithFileSet(file, fset)
			if err != nil {
				t.Fatal(err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("applied service missing %q, got:\n%s", want, got)
				}
			}
			// The gst model import must be pruned when the request type moves
			// back to a business type, otherwise the file no longer compiles.
			for _, missing := range tt.wantMissing {
				if strings.Contains(got, missing) {
					t.Errorf("applied service still contains %q, got:\n%s", missing, got)
				}
			}
			if strings.Count(got, GstModelImportPath) > 1 {
				t.Errorf("gst model import duplicated, got:\n%s", got)
			}
		})
	}
}
