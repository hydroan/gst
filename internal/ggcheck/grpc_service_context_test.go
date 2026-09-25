package ggcheck_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// TestGRPCServiceContextFlagsHTTPOnlyCalls pins that the service packages of
// a model declaring GRPC() are held to the ServiceContext methods both
// transports serve: a call to one HTTP alone serves is reported at its line,
// from a service method or a helper of the package alike, under the name
// the function gives the context, while a service of a model served over
// HTTP only may call anything.
func TestGRPCServiceContextFlagsHTTPOnlyCalls(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "group.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Group struct {
	Name string `+"`"+`json:"name" pb:"11"`+"`"+`

	model.Base
}

func (Group) TableName() string { return "groups" }

func (Group) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("groups")
	dsl.Create(func() {
		dsl.Service()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "model", "plain.go"), `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Plain struct {
	Name string `+"`"+`json:"name"`+"`"+`

	model.Base
}

func (Plain) TableName() string { return "plains" }

func (Plain) Design() {
	dsl.Migrate()
	dsl.Endpoint("plains")
	dsl.Create(func() {
		dsl.Service()
	})
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "group", "create.go"), `package group

import (
	"net/http"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Group, *model.Group, *model.Group]
}

func (c *Creator) Create(ctx *gst.ServiceContext, group *model.Group) (*model.Group, error) {
	_ = ctx.Param("id")
	ctx.SetCookie(&http.Cookie{Name: "seen"})
	return group, nil
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "group", "helper.go"), `package group

import "github.com/hydroan/gst"

func reply(sc *gst.ServiceContext, body []byte) {
	sc.Data(200, "text/plain", body)
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "plain", "create.go"), `package plain

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Plain, *model.Plain, *model.Plain]
}

func (c *Creator) Create(ctx *gst.ServiceContext, plain *model.Plain) (*model.Plain, error) {
	ctx.Data(200, "text/plain", nil)
	return plain, nil
}
`)

	violations := runCheck(ggcheck.GRPCServiceContext)

	want := []string{
		"service/group/create.go:17: calls ctx.SetCookie, which only HTTP serves; the model Group is served over gRPC as well, so keep its services to what both transports provide",
		"service/group/helper.go:6: calls sc.Data, which only HTTP serves; the model Group is served over gRPC as well, so keep its services to what both transports provide",
	}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
