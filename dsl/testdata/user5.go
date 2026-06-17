package model

import (
	. "github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type User5 struct {
	Name string
	Addr string

	model.Base
}
