package helloworld

import (
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

var _ types.Module[*Helloworld, *Req, *Rsp] = (*Module)(nil)

// Helloworld is the model definition.
type Helloworld struct {
	model.Empty
}

// Req is the custom request type.
type Req struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

// Rsp is the custom response type.
type Rsp struct {
	Field3 string `json:"field3"`
	Field4 int    `json:"field4"`
}

// Service implements the `types.Service` interface.
type Service struct {
	service.Base[*Helloworld, *Req, *Rsp]
}

// Module implements the `types.Module` interface.
type Module struct{}

func (Module) Service() types.Service[*Helloworld, *Req, *Rsp] {
	return &Service{}
}
func (Module) Pub() bool     { return false }
func (Module) Route() string { return "hello-world" }
func (Module) Param() string { return "id" }
