package helloworld

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Helloworld2, *Helloworld2, *Helloworld2] = (*Module2)(nil)

type Helloworld2 struct {
	Before string `json:"before" query:"before"`
	After  string `json:"after" query:"after"`

	model.Base
}

type Service2 struct {
	service.Base[*Helloworld2, *Helloworld2, *Helloworld2]
}

type Module2 struct{}

func (*Module2) Service() types.Service[*Helloworld2, *Helloworld2, *Helloworld2] {
	return &Service2{}
}

func (*Module2) Route() string {
	return "hello-world2"
}

// Param returns the route parameter identifier.
// returns empty string to use default "id".
func (*Module2) Param() string {
	return ""
}

func (*Module2) Pub() bool {
	return false
}
