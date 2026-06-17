package model

import "github.com/hydroan/gst/model"

type Group struct {
	Name     string
	NumUsers int

	model.Base
}
