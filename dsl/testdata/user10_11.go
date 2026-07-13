package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User10 struct {
	Name string

	model.AutoBase
}

func (User10) Design() {
	dsl.Migrate()
}

type User11 struct {
	Name string

	model.Base
}
