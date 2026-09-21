package gen_test

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/hydroan/gst/consts"
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/internal/codegen/gen"
	"github.com/kr/pretty"
)

var dataServiceUserCreate string

func init() {
	var data []byte
	var err error
	if data, err = os.ReadFile("./testdata/service/user_create.go"); err != nil {
		panic(err)
	}
	dataServiceUserCreate = string(data)
}

func TestApplyServiceFile(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		action         *dsl.Action
		servicePkgName string
		want           string
	}{
		{
			name: "user_create_with_payload_result",
			code: dataServiceUserCreate,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "service",
			want: `package service

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.UserReq, *model.UserRsp]
}

func (u *user) Create(ctx *gst.ServiceContext, req *model.UserReq) (rsp *model.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create after")
	return nil
}
`,
		},
		{
			name: "user_create_no_payload_result",
			code: dataServiceUserCreate,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*User",
				Result:  "*User",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "service",
			want: `package service

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create after")
	return nil
}
`,
		},
		{
			// Bare action type names (the declared form of slice and map
			// action types) are transcribed as value types.
			name: "bare_action_names_transcribed",
			code: dataServiceUserCreate,
			action: &dsl.Action{
				Enabled: true,
				Payload: "UserReq",
				Result:  "UserRsp",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "service",
			want: `package service

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, model.UserReq, model.UserRsp]
}

func (u *user) Create(ctx *gst.ServiceContext, req model.UserReq) (rsp model.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *gst.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create after")
	return nil
}
`,
		},
		{
			name: "package_name_correction_lowercase",
			code: `package wrongname

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*User",
				Result:  "*User",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "callback",
			want: `package callback

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "rename_struct_and_receiver_with_filename",
			code: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}

func (c *Creator) CreateBefore(ctx *gst.ServiceContext, record *sample.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create before")
	return nil
}

func (c *Creator) CreateAfter(ctx *gst.ServiceContext, record *sample.Record) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create after")
	return nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*Record",
				Result:   "*Record",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			servicePkgName: "record",
			want: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (u *Upload) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}

func (u *Upload) CreateBefore(ctx *gst.ServiceContext, record *sample.Record) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create before")
	return nil
}

func (u *Upload) CreateAfter(ctx *gst.ServiceContext, record *sample.Record) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create after")
	return nil
}
`,
		},
		{
			name: "rename_struct_and_receiver_with_filename_and_payload",
			code: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*RecordReq",
				Result:   "*RecordRsp",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			servicePkgName: "record",
			want: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.RecordReq, *sample.RecordRsp]
}

func (u *Upload) Create(ctx *gst.ServiceContext, req *sample.RecordReq) (rsp *sample.RecordRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
		},
		{
			name: "no_rename_when_filename_not_set",
			code: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*Record",
				Result:  "*Record",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "record",
			want: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
		},
		{
			name: "no_change_when_struct_and_receiver_already_match",
			code: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (u *Upload) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*Record",
				Result:   "*Record",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			servicePkgName: "record",
			want: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.Record, *sample.Record]
}

func (u *Upload) Create(ctx *gst.ServiceContext, req *sample.Record) (rsp *sample.Record, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}
`,
		},
		{
			name: "rename_receiver_when_struct_already_matches",
			code: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.RecordReq, *sample.RecordRsp]
}

func (r *Upload) Create(ctx *gst.ServiceContext, req *sample.RecordReq) (rsp *sample.RecordRsp, err error) {
	log := r.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}

func (r *Upload) CreateBefore(ctx *gst.ServiceContext, record *sample.Record) error {
	log := r.WithContext(ctx, ctx.Phase())
	log.Info("record create before")
	return nil
}

func (r *Upload) CreateAfter(ctx *gst.ServiceContext, record *sample.Record) error {
	log := r.WithContext(ctx, ctx.Phase())
	log.Info("record create after")
	return nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*RecordReq",
				Result:   "*RecordRsp",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			servicePkgName: "record",
			want: `package record

import (
	"helloworld/model/sample"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Upload struct {
	service.Base[*sample.Record, *sample.RecordReq, *sample.RecordRsp]
}

func (u *Upload) Create(ctx *gst.ServiceContext, req *sample.RecordReq) (rsp *sample.RecordRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create")
	return rsp, nil
}

func (u *Upload) CreateBefore(ctx *gst.ServiceContext, record *sample.Record) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create before")
	return nil
}

func (u *Upload) CreateAfter(ctx *gst.ServiceContext, record *sample.Record) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("record create after")
	return nil
}
`,
		},
		{
			name: "package_name_correction_underscore",
			code: `package sample_item

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type sampleItem struct {
	service.Base[*model.SampleItem, *model.SampleItem, *model.SampleItem]
}

func (s *sampleItem) Create(ctx *gst.ServiceContext, req *model.SampleItem) (rsp *model.SampleItem, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*SampleItem",
				Result:  "*SampleItem",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "sampleitem",
			want: `package sampleitem

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type sampleItem struct {
	service.Base[*model.SampleItem, *model.SampleItem, *model.SampleItem]
}

func (s *sampleItem) Create(ctx *gst.ServiceContext, req *model.SampleItem) (rsp *model.SampleItem, err error) {
	return rsp, nil
}
`,
		},
		{
			// Regression test for an incident where isServiceMethod4 matched a hand-written
			// helper by shape alone: Patcher.validate has the same
			// (ctx *gst.ServiceContext, req *pkg.Req) (*pkg.X, error) shape as the real
			// Patch action method, so it was mistaken for the action method and rewritten
			// in place, corrupting its return type and breaking the build. applyServiceMethod4
			// must only rewrite the function whose name matches action.Phase.MethodName().
			name: "does_not_rewrite_non_action_function_with_same_shape",
			code: `package samplerecord

import (
	"helloworld/model/group"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Patcher struct {
	service.Base[*group.SampleRecord, *group.SampleRecordPatchReq, *group.SampleRecordPatchRsp]
}

func (r *Patcher) Patch(ctx *gst.ServiceContext, req *group.SampleRecordPatchReq) (rsp *group.SampleRecordPatchRsp, err error) {
	return rsp, nil
}

func (r *Patcher) validate(ctx *gst.ServiceContext, req *group.SampleRecordPatchReq) (*group.SampleRecord, error) {
	return nil, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*SampleRecordPatchReq",
				Result:  "*SampleRecordPatchRsp",
				Phase:   consts.PHASE_PATCH,
			},
			servicePkgName: "samplerecord",
			want: `package samplerecord

import (
	"helloworld/model/group"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Patcher struct {
	service.Base[*group.SampleRecord, *group.SampleRecordPatchReq, *group.SampleRecordPatchRsp]
}

func (r *Patcher) Patch(ctx *gst.ServiceContext, req *group.SampleRecordPatchReq) (rsp *group.SampleRecordPatchRsp, err error) {
	return rsp, nil
}

func (r *Patcher) validate(ctx *gst.ServiceContext, req *group.SampleRecordPatchReq) (*group.SampleRecord, error) {
	return nil, nil
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			gen.ApplyServiceFile(file, tt.action, tt.servicePkgName)
			got, err := gen.FormatNodeExtra(file)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestApplyServiceFileEmptyPayload(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		action         *dsl.Action
		servicePkgName string
		wantContains   []string
		wantAbsent     []string
	}{
		{
			name: "switch_business_req_to_empty_payload_adds_gst_model_import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*group.Group, *group.GroupListReq, *group.GroupListRsp]
}

func (g *Lister) List(ctx *gst.ServiceContext, req *group.GroupListReq) (rsp *group.GroupListRsp, err error) {
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
			name: "switch_empty_payload_back_to_model_removes_gst_model_import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}

func (g *Lister) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {
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
			wantAbsent: []string{
				`"github.com/hydroan/gst/model"`,
			},
		},
		{
			name: "switch_to_empty_payload_in_root_model_package_uses_gstmodel_alias",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Getter struct {
	service.Base[*model.User, *model.UserGetReq, *model.UserGetRsp]
}

func (u *Getter) Get(ctx *gst.ServiceContext, req *model.UserGetReq) (rsp *model.UserGetRsp, err error) {
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
			name: "empty_payload_apply_is_idempotent_for_existing_import",
			code: `package group

import (
	"helloworld/model/group"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*group.Group, *model.Empty, *group.GroupListRsp]
}

func (g *Lister) List(ctx *gst.ServiceContext, req *model.Empty) (rsp *group.GroupListRsp, err error) {
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

			gen.ApplyServiceFile(file, tt.action, tt.servicePkgName)

			got, err := gen.FormatNodeExtraWithFileSet(file, fset)
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
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("applied service still contains %q, got:\n%s", absent, got)
				}
			}
			if strings.Count(got, gen.GstModelImportPath) > 1 {
				t.Errorf("gst model import duplicated, got:\n%s", got)
			}
		})
	}
}

func TestApplyServiceFileWithModelSync(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		action         *dsl.Action
		servicePkgName string
		modelInfo      *gen.ModelInfo
		want           string
	}{
		{
			name: "update_import_and_package_references",
			code: `package user

import (
	"helloworld/model/identity"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*identity.User, *identity.UserReq, *identity.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *identity.UserReq) (rsp *identity.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "user",
			modelInfo: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelFileDir: "model/auth",
				ModelPkgName: "auth",
				ModelName:    "User",
			},
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	return rsp, nil
}
`,
		},
		{
			name: "no_change_if_import_already_correct",
			code: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "user",
			modelInfo: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelFileDir: "model/auth",
				ModelPkgName: "auth",
				ModelName:    "User",
			},
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "update_import_with_alias",
			code: `package user

import (
	oldpkg "helloworld/model/identity"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*oldpkg.User, *oldpkg.UserReq, *oldpkg.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *oldpkg.UserReq) (rsp *oldpkg.UserRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
				Phase:   consts.PHASE_CREATE,
			},
			servicePkgName: "user",
			modelInfo: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelFileDir: "model/auth",
				ModelPkgName: "auth",
				ModelName:    "User",
			},
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *gst.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "do_not_update_unrelated_model_imports",
			code: `package debug

import (
	"helloworld/model/auth"
	"helloworld/model/sample/item"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*auth.Debug, *auth.Debug, *auth.Debug]
}

func (d *Lister) List(ctx *gst.ServiceContext, req *auth.Debug) (rsp *auth.Debug, err error) {
	items := make([]*item.Entry, 0)
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*Debug",
				Result:  "*Debug",
				Phase:   consts.PHASE_LIST,
			},
			servicePkgName: "debug",
			modelInfo: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelFileDir: "model/auth",
				ModelPkgName: "auth",
				ModelName:    "Debug",
			},
			want: `package debug

import (
	"helloworld/model/auth"
	"helloworld/model/sample/item"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Lister struct {
	service.Base[*auth.Debug, *auth.Debug, *auth.Debug]
}

func (d *Lister) List(ctx *gst.ServiceContext, req *auth.Debug) (rsp *auth.Debug, err error) {
	items := make([]*item.Entry, 0)
	return rsp, nil
}
`,
		},
		{
			name: "update_stale_service_model_type",
			code: `package debug

import (
	"helloworld/model/debug"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Ping struct {
	service.Base[*debug.Ping, *debug.Debug, *debug.PingRsp]
}

func (p *Ping) Get(ctx *gst.ServiceContext, req *debug.Debug) (rsp *debug.PingRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*Debug",
				Result:  "*PingRsp",
				// Filename keeps the struct name "Ping" canonical for the
				// action, so this case exercises only the stale model type
				// sync and not the role name restoration.
				Filename: "ping",
				Phase:    consts.PHASE_GET,
			},
			servicePkgName: "debug",
			modelInfo: &gen.ModelInfo{
				ModulePath:   "helloworld",
				ModelFileDir: "model/debug",
				ModelPkgName: "debug",
				ModelName:    "Debug",
			},
			want: `package debug

import (
	"helloworld/model/debug"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Ping struct {
	service.Base[*debug.Debug, *debug.Debug, *debug.PingRsp]
}

func (p *Ping) Get(ctx *gst.ServiceContext, req *debug.Debug) (rsp *debug.PingRsp, err error) {
	return rsp, nil
}
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "", tt.code, parser.ParseComments)
			if err != nil {
				t.Error(err)
				return
			}
			gen.ApplyServiceFileWithModelSync(file, tt.action, tt.servicePkgName, tt.modelInfo)
			got, err := gen.FormatNodeExtra(file)
			if err != nil {
				t.Error(err)
				return
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", pretty.Sprintf("% #v", got), pretty.Sprintf("% #v", tt.want))
			}
		})
	}
}

func TestApplyServiceFileWithModelSyncForcesCanonicalServiceStruct(t *testing.T) {
	modelInfo := &gen.ModelInfo{
		ModulePath:   "helloworld",
		ModelFileDir: "model",
		ModelPkgName: "model",
		ModelName:    "User",
	}
	exportAction := &dsl.Action{
		Enabled: true,
		Service: true,
		Payload: "*User",
		Result:  "*User",
		Phase:   consts.PHASE_EXPORT,
	}

	tests := []struct {
		name         string
		code         string
		action       *dsl.Action
		wantChanged  bool
		wantContains []string // substrings that must appear in the rewritten file
		wantAbsent   []string // substrings that must not appear in the rewritten file
	}{
		{
			// A hand edit replaced the service.Base embedding with another
			// service struct and dropped the service import; the struct body is
			// generated code, so it is forced back to the canonical single
			// embedding and the extra field is discarded.
			name: "forces_body_with_foreign_embedding",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
)

type Exporter struct {
	// service.Base[*model.User, *model.User, *model.User]
	Lister
}

func (e *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: true,
			wantContains: []string{
				"service.Base[*model.User, *model.User, *model.User]",
				`"github.com/hydroan/gst/service"`,
			},
			wantAbsent: []string{"Lister"},
		},
		{
			// A malformed service.Base embedding (wrong arity) is replaced by
			// the canonical one instead of gaining a duplicate next to it.
			name: "forces_body_with_malformed_embedding",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Exporter struct {
	service.Base[*model.User]
}

func (e *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: true,
			wantContains: []string{
				"service.Base[*model.User, *model.User, *model.User]",
			},
		},
		{
			// A struct that was deleted entirely is regenerated, so gg gen
			// always converges on a registrable service struct.
			name: "restores_deleted_struct",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
)

func exportHeaders(users ...*model.User) []string { return nil }

var _ = gst.ServiceContext{}
`,
			action:      exportAction,
			wantChanged: true,
			wantContains: []string{
				"type Exporter struct",
				"service.Base[*model.User, *model.User, *model.User]",
				`"github.com/hydroan/gst/service"`,
			},
		},
		{
			// A struct that already has the canonical body needs no rewrite.
			name: "keeps_canonical_struct_untouched",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Exporter struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (e *Exporter) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
	return data, err
}
`,
			action:      exportAction,
			wantChanged: false,
			wantContains: []string{
				"service.Base[*model.User, *model.User, *model.User]",
			},
		},
		{
			// A hand edit renamed the struct of a Filename-less action away
			// from the phase role name. No rename path covers this case, yet
			// the generated registration code still references the role name,
			// so the struct and its method receivers are restored to the
			// canonical name; receiver variable names and method bodies are
			// user-visible code and stay untouched.
			name: "restores_renamed_struct_without_filename",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Mangled struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (x *Mangled) Export(ctx *gst.ServiceContext, users ...*model.User) (data []byte, err error) {
	data = append(data, 'a')
	return data, err
}
`,
			action:      exportAction,
			wantChanged: true,
			wantContains: []string{
				"type Exporter struct",
				"func (x *Exporter) Export",
				"data = append(data, 'a')",
			},
			wantAbsent: []string{"Mangled"},
		},
		{
			// With Filename set, a canonical struct still carrying the old role
			// name belongs to the rename path, not the force-rewrite path.
			name: "skips_rewrite_when_filename_rename_applies",
			code: `package user

import (
	"helloworld/model"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

type Creator struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (c *Creator) Create(ctx *gst.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, err
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Service:  true,
				Payload:  "*User",
				Result:   "*User",
				Filename: "upload",
				Phase:    consts.PHASE_CREATE,
			},
			wantChanged: true,
			wantContains: []string{
				"type Upload struct",
				"service.Base[*model.User, *model.User, *model.User]",
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

			changed := gen.ApplyServiceFileWithModelSync(file, tt.action, "user", modelInfo)
			if changed != tt.wantChanged {
				t.Errorf("ApplyServiceFileWithModelSync changed = %v, want %v", changed, tt.wantChanged)
			}

			got, err := gen.FormatNodeExtra(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Expected to find %q in rewritten code, but got:\n%s", want, got)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(got, absent) {
					t.Errorf("Expected %q to be removed from rewritten code, but got:\n%s", absent, got)
				}
			}
			if count := strings.Count(got, "service.Base["); count != 1 {
				t.Errorf("Expected exactly one service.Base embedding, found %d in:\n%s", count, got)
			}
		})
	}
}
