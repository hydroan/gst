package modelauthz

import (
	"context"

	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/util"
	"go.uber.org/zap/zapcore"
)

type Permission struct {
	Resource string `json:"resource,omitempty" schema:"resource"`
	Action   string `json:"action,omitempty" schema:"action"`

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
