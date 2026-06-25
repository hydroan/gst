package gen

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/hydroan/gst/dsl"
	"github.com/kr/pretty"
)

func TestApplyServiceFile(t *testing.T) {
	tests := []struct {
		name           string // description of this test case
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
				Payload: "UserReq",
				Result:  "UserRsp",
			},
			servicePkgName: "service",
			want: `package service

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type user struct {
	service.Base[*model.User, model.UserReq, model.UserRsp]
}

func (u *user) Create(ctx *types.ServiceContext, req model.UserReq) (rsp model.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *types.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *types.ServiceContext, user *model.User) error {
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
				Payload: "User",
				Result:  "User",
			},
			servicePkgName: "service",
			want: `package service

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type user struct {
	service.Base[*model.User, model.User, model.User]
}

func (u *user) Create(ctx *types.ServiceContext, req model.User) (rsp model.User, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create")
	return rsp, nil
}

func (u *user) CreateBefore(ctx *types.ServiceContext, user *model.User) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("user create before")
	return nil
}

func (u *user) CreateAfter(ctx *types.ServiceContext, user *model.User) error {
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

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*User",
				Result:  "*User",
			},
			servicePkgName: "callback",
			want: `package callback

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type user struct {
	service.Base[*model.User, *model.User, *model.User]
}

func (u *user) Create(ctx *types.ServiceContext, req *model.User) (rsp *model.User, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "rename_struct_and_receiver_with_filename",
			code: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}

func (c *Creator) CreateBefore(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create before")
	return nil
}

func (c *Creator) CreateAfter(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create after")
	return nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*Attachment",
				Result:   "*Attachment",
				Filename: "upload",
			},
			servicePkgName: "attachment",
			want: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (u *Upload) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}

func (u *Upload) CreateBefore(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create before")
	return nil
}

func (u *Upload) CreateAfter(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create after")
	return nil
}
`,
		},
		{
			name: "rename_struct_and_receiver_with_filename_and_payload",
			code: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*AttachmentReq",
				Result:   "*AttachmentRsp",
				Filename: "upload",
			},
			servicePkgName: "attachment",
			want: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.AttachmentReq, *shared.AttachmentRsp]
}

func (u *Upload) Create(ctx *types.ServiceContext, req *shared.AttachmentReq) (rsp *shared.AttachmentRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
		},
		{
			name: "no_rename_when_filename_not_set",
			code: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*Attachment",
				Result:  "*Attachment",
			},
			servicePkgName: "attachment",
			want: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (c *Creator) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := c.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
		},
		{
			name: "no_change_when_struct_and_receiver_already_match",
			code: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (u *Upload) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*Attachment",
				Result:   "*Attachment",
				Filename: "upload",
			},
			servicePkgName: "attachment",
			want: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.Attachment, *shared.Attachment]
}

func (u *Upload) Create(ctx *types.ServiceContext, req *shared.Attachment) (rsp *shared.Attachment, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}
`,
		},
		{
			name: "rename_receiver_when_struct_already_matches",
			code: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.AttachmentReq, *shared.AttachmentRsp]
}

func (a *Upload) Create(ctx *types.ServiceContext, req *shared.AttachmentReq) (rsp *shared.AttachmentRsp, err error) {
	log := a.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}

func (a *Upload) CreateBefore(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := a.WithContext(ctx, ctx.Phase())
	log.Info("attachment create before")
	return nil
}

func (a *Upload) CreateAfter(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := a.WithContext(ctx, ctx.Phase())
	log.Info("attachment create after")
	return nil
}
`,
			action: &dsl.Action{
				Enabled:  true,
				Payload:  "*AttachmentReq",
				Result:   "*AttachmentRsp",
				Filename: "upload",
			},
			servicePkgName: "attachment",
			want: `package attachment

import (
	"helloworld/model/shared"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Upload struct {
	service.Base[*shared.Attachment, *shared.AttachmentReq, *shared.AttachmentRsp]
}

func (u *Upload) Create(ctx *types.ServiceContext, req *shared.AttachmentReq) (rsp *shared.AttachmentRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create")
	return rsp, nil
}

func (u *Upload) CreateBefore(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create before")
	return nil
}

func (u *Upload) CreateAfter(ctx *types.ServiceContext, attachment *shared.Attachment) error {
	log := u.WithContext(ctx, ctx.Phase())
	log.Info("attachment create after")
	return nil
}
`,
		},
		{
			name: "package_name_correction_configsetting",
			code: `package config_setting

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type configSetting struct {
	service.Base[*model.ConfigSetting, *model.ConfigSetting, *model.ConfigSetting]
}

func (c *configSetting) Create(ctx *types.ServiceContext, req *model.ConfigSetting) (rsp *model.ConfigSetting, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*ConfigSetting",
				Result:  "*ConfigSetting",
			},
			servicePkgName: "configsetting",
			want: `package configsetting

import (
	"helloworld/model"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type configSetting struct {
	service.Base[*model.ConfigSetting, *model.ConfigSetting, *model.ConfigSetting]
}

func (c *configSetting) Create(ctx *types.ServiceContext, req *model.ConfigSetting) (rsp *model.ConfigSetting, err error) {
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
			ApplyServiceFile(file, tt.action, tt.servicePkgName)
			got, err := FormatNodeExtra(file)
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

func TestApplyServiceFileWithModelSync(t *testing.T) {
	tests := []struct {
		name                   string // description of this test case
		code                   string
		action                 *dsl.Action
		servicePkgName         string
		correctModelImportPath string
		correctModelPkgName    string
		want                   string
	}{
		{
			name: "update_import_and_package_references",
			code: `package user

import (
	"helloworld/model/identity"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*identity.User, *identity.UserReq, *identity.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *identity.UserReq) (rsp *identity.UserRsp, err error) {
	log := u.WithContext(ctx, ctx.Phase())
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			servicePkgName:         "user",
			correctModelImportPath: "helloworld/model/auth",
			correctModelPkgName:    "auth",
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
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

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			servicePkgName:         "user",
			correctModelImportPath: "helloworld/model/auth",
			correctModelPkgName:    "auth",
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "update_import_with_alias",
			code: `package user

import (
	oldpkg "helloworld/model/identity"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*oldpkg.User, *oldpkg.UserReq, *oldpkg.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *oldpkg.UserReq) (rsp *oldpkg.UserRsp, err error) {
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*UserReq",
				Result:  "*UserRsp",
			},
			servicePkgName:         "user",
			correctModelImportPath: "helloworld/model/auth",
			correctModelPkgName:    "auth",
			want: `package user

import (
	"helloworld/model/auth"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Creator struct {
	service.Base[*auth.User, *auth.UserReq, *auth.UserRsp]
}

func (u *Creator) Create(ctx *types.ServiceContext, req *auth.UserReq) (rsp *auth.UserRsp, err error) {
	return rsp, nil
}
`,
		},
		{
			name: "do_not_update_unrelated_model_imports",
			code: `package debug

import (
	"helloworld/model/auth"
	"helloworld/model/config/namespace"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*auth.Debug, *auth.Debug, *auth.Debug]
}

func (d *Lister) List(ctx *types.ServiceContext, req *auth.Debug) (rsp *auth.Debug, err error) {
	files := make([]*namespace.File, 0)
	return rsp, nil
}
`,
			action: &dsl.Action{
				Enabled: true,
				Payload: "*Debug",
				Result:  "*Debug",
			},
			servicePkgName:         "debug",
			correctModelImportPath: "helloworld/model/auth",
			correctModelPkgName:    "auth",
			want: `package debug

import (
	"helloworld/model/auth"
	"helloworld/model/config/namespace"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

type Lister struct {
	service.Base[*auth.Debug, *auth.Debug, *auth.Debug]
}

func (d *Lister) List(ctx *types.ServiceContext, req *auth.Debug) (rsp *auth.Debug, err error) {
	files := make([]*namespace.File, 0)
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
			ApplyServiceFileWithModelSync(file, tt.action, tt.servicePkgName, tt.correctModelImportPath, tt.correctModelPkgName)
			got, err := FormatNodeExtra(file)
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
