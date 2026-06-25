package modelauthz

import (
	"context"
	"fmt"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
)

type Permission struct {
	Resource string  `json:"resource,omitempty" schema:"resource"`
	Action   string  `json:"action,omitempty" schema:"action"`
	Remark   *string `json:"remark,omitempty" gorm:"size:10240" schema:"remark"`

	model.Base
}

func (Permission) Design() {
	dsl.Migrate(true)
	dsl.Route("authz/permissions", func() {
		dsl.List(func() {})
		dsl.Get(func() {})
	})
}

func (p *Permission) Purge() bool { return true }

func (p *Permission) CreateBefore(context.Context) error {
	p.SetID(util.HashID(p.Resource, p.Action))
	p.Remark = new(fmt.Sprintf("%s %s", p.Action, p.Resource))
	return nil
}

func (p *Permission) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	if p == nil {
		return nil
	}
	enc.AddString("resource", p.Resource)
	enc.AddString("action", p.Action)
	_ = enc.AddObject("base", &p.Base)
	return nil
}
