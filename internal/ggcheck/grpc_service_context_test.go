package ggcheck_test

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
)

// grpcCheckModel declares Entry, served over gRPC, with the actions each
// case of the check needs: a Create with a service, an SSE and an Export
// HTTP alone serves, and a Stream.
const grpcCheckModel = `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type Entry struct {
	Name string ` + "`" + `json:"name" pb:"11"` + "`" + `

	model.Base
}

func (Entry) TableName() string { return "entries" }

func (Entry) Design() {
	dsl.GRPC()
	dsl.Migrate()
	dsl.Endpoint("entries")
	dsl.Create(func() {
		dsl.Service()
	})
	dsl.SSE(func() {
		dsl.Service()
	})
	dsl.Export(func() {
		dsl.Service()
	})
	dsl.Route("entries/chat", func() {
		dsl.Stream(func() {
			dsl.Service("chat")
			dsl.StreamingPayload[*Entry]()
			dsl.StreamingResult[*Entry]()
		})
	})
}
`

// grpcCheckCreate is the Create service of Entry: it calls one method only
// HTTP serves itself and reaches another through a helper of its package.
const grpcCheckCreate = `package entry

import (
	"net/http"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (c *Creator) Create(ctx *gst.ServiceContext, entry *model.Entry) (*model.Entry, error) {
	_ = ctx.Param("id")
	ctx.SetCookie(&http.Cookie{Name: "seen"})
	reply(ctx, nil)
	return entry, nil
}
`

const grpcCheckHelper = `package entry

import "github.com/hydroan/gst"

func reply(sc *gst.ServiceContext, body []byte) {
	sc.Data(200, "text/plain", body)
}
`

// TestGRPCServiceContextFlagsHTTPOnlyCalls pins what the check reports for
// a model served over gRPC and what it leaves alone: a call of a method only
// HTTP serves is reported at its line, from the service method, from a hook
// of the service type in another file, from a helper of the package or of
// another package of the project, from a Stream service, and under the
// name the function gives the context, the framework package imported under
// an alias included; left alone are the services of a model served over
// HTTP alone and of the SSE and Export actions, the helpers only those
// reach, a method of another type sharing the name of one a gRPC call
// reaches, and a local variable named like the context. A file of the
// service package the parser refuses is reported once.
func TestGRPCServiceContextFlagsHTTPOnlyCalls(t *testing.T) {
	violation := func(path, source, fragment, call string) string {
		return fmt.Sprintf("%s:%d: calls %s, which only HTTP serves; the model Entry is served over gRPC as well, so keep its services to what both transports provide", path, sourceLine(t, source, fragment), call)
	}
	base := []string{
		violation("service/entry/create.go", grpcCheckCreate, "ctx.SetCookie", "ctx.SetCookie"),
		violation("service/entry/helper.go", grpcCheckHelper, "sc.Data", "sc.Data"),
	}
	hooks := `package entry

import (
	"net/http"

	"github.com/hydroan/gst"
	"tmpapp/model"
)

func (c *Creator) CreateAfter(ctx *gst.ServiceContext, entry *model.Entry) error {
	ctx.SetCookie(&http.Cookie{Name: "created"})
	return nil
}
`
	cookie := `package cookie

import (
	"net/http"

	"github.com/hydroan/gst"
)

func Set(ctx *gst.ServiceContext, name string) {
	ctx.SetCookie(&http.Cookie{Name: name})
}
`
	createThroughCookie := `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/internal/cookie"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (c *Creator) Create(ctx *gst.ServiceContext, entry *model.Entry) (*model.Entry, error) {
	cookie.Set(ctx, "seen")
	return entry, nil
}
`
	sends := `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/sse"
)

func (c *Creator) send(ctx *gst.ServiceContext) { _ = ctx.Param("id") }

func (s *Streamer) send(ctx *gst.ServiceContext) error {
	return ctx.SSE(func(conn *sse.Conn) error { return nil })
}
`
	createSending := `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (c *Creator) Create(ctx *gst.ServiceContext, entry *model.Entry) (*model.Entry, error) {
	c.send(ctx)
	return entry, nil
}
`
	chat := `package entry

import (
	"net/http"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/grpc"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Chatter struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (c *Chatter) Stream(ctx *gst.ServiceContext, stream *grpc.BidiStream[*model.Entry, *model.Entry]) error {
	ctx.SetCookie(&http.Cookie{Name: "chat"})
	return nil
}
`
	export := `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Exporter struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (e *Exporter) Export(ctx *gst.ServiceContext) ([]byte, error) {
	ctx.Data(200, "text/csv", nil)
	return nil, nil
}
`
	aliased := `package entry

import (
	"net/http"

	gstpkg "github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (c *Creator) Create(sc *gstpkg.ServiceContext, entry *model.Entry) (*model.Entry, error) {
	sc.SetCookie(&http.Cookie{Name: "seen"})
	return entry, nil
}
`
	shadowed := `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Creator struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

type spy struct{}

func (spy) Data(int, string, []byte) {}

func (c *Creator) Create(sc *gst.ServiceContext, entry *model.Entry) (*model.Entry, error) {
	ctx := spy{}
	ctx.Data(200, "text/plain", nil)
	return entry, nil
}
`
	tests := []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{
			name: "the service method and a helper of its package",
			want: base,
		},
		{
			name:  "a hook of the service type in another file",
			files: map[string]string{"service/entry/create_hooks.go": hooks},
			want:  append(slices.Clone(base), violation("service/entry/create_hooks.go", hooks, "ctx.SetCookie", "ctx.SetCookie")),
		},
		{
			name:  "a helper of another package of the project",
			files: map[string]string{"service/entry/create.go": createThroughCookie, "internal/cookie/cookie.go": cookie},
			want:  []string{violation("internal/cookie/cookie.go", cookie, "ctx.SetCookie", "ctx.SetCookie")},
		},
		{
			name:  "a method of another type sharing the name of one reached",
			files: map[string]string{"service/entry/create.go": createSending, "service/entry/send.go": sends},
			want:  nil,
		},
		{
			name:  "a Stream service",
			files: map[string]string{"service/entry/chat.go": chat},
			want:  append(slices.Clone(base), violation("service/entry/chat.go", chat, "ctx.SetCookie", "ctx.SetCookie")),
		},
		{
			name:  "an Export service",
			files: map[string]string{"service/entry/export.go": export},
			want:  base,
		},
		{
			name:  "the framework package under an alias",
			files: map[string]string{"service/entry/create.go": aliased},
			want:  []string{violation("service/entry/create.go", aliased, "sc.SetCookie", "sc.SetCookie")},
		},
		{
			name:  "a local named like the context",
			files: map[string]string{"service/entry/create.go": shadowed},
			want:  nil,
		},
		{
			name:  "a file the parser refuses",
			files: map[string]string{"service/entry/broken.go": "package entry\n\nfunc {\n"},
			want:  append(slices.Clone(base), "service/entry/broken.go:3:6: expected 'IDENT', found '{'"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectDir := t.TempDir()
			t.Chdir(projectDir)
			writeGRPCCheckProject(t, projectDir)
			for path, content := range tt.files {
				writeCheckFile(t, filepath.Join(projectDir, filepath.FromSlash(path)), content)
			}

			violations := runCheck(ggcheck.GRPCServiceContext)

			slices.Sort(violations)
			slices.Sort(tt.want)
			if !slices.Equal(violations, tt.want) {
				t.Fatalf("violations = %#v, want %#v", violations, tt.want)
			}
		})
	}
}

// writeGRPCCheckProject writes the project the cases of the check start
// from: the Entry model, its Create service and helper, its SSE service and
// the helper only that reaches, and the Plain model served over HTTP alone.
func writeGRPCCheckProject(t *testing.T, projectDir string) {
	t.Helper()
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	writeCheckFile(t, filepath.Join(projectDir, "model", "entry.go"), grpcCheckModel)
	writeCheckFile(t, filepath.Join(projectDir, "service", "entry", "create.go"), grpcCheckCreate)
	writeCheckFile(t, filepath.Join(projectDir, "service", "entry", "helper.go"), grpcCheckHelper)
	// The SSE action of the model is HTTP only, so its service may call
	// ctx.SSE, and so may a helper only the SSE service reaches: no gRPC
	// call gets there.
	writeCheckFile(t, filepath.Join(projectDir, "service", "entry", "sse.go"), `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Streamer struct {
	service.Base[*model.Entry, *model.Entry, *model.Entry]
}

func (s *Streamer) SSE(ctx *gst.ServiceContext, entry *model.Entry) error {
	return render(ctx)
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "service", "entry", "render.go"), `package entry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/sse"
)

func render(ctx *gst.ServiceContext) error {
	return ctx.SSE(func(conn *sse.Conn) error { return nil })
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
}

// TestGRPCServiceContextNamesTheModelWhoseCallReaches pins the model a
// violation is attributed to when two models share a service package
// through Flatten: the one whose gRPC call reaches the function, not the
// first model of the package.
func TestGRPCServiceContextNamesTheModelWhoseCallReaches(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	for _, name := range []string{"Item", "Record"} {
		lower := strings.ToLower(name)
		writeCheckFile(t, filepath.Join(projectDir, "model", "pkg", lower+".go"), `package pkg

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
)

type `+name+` struct {
	model.Empty
}

func (`+name+`) Design() {
	dsl.GRPC()
	dsl.Route("`+lower+`s/seal", func() {
		dsl.Create(func() {
			dsl.Flatten()
			dsl.Service("seal`+name+`")
			dsl.Payload[*`+name+`]()
			dsl.Result[*`+name+`]()
		})
	})
}
`)
	}
	writeCheckFile(t, filepath.Join(projectDir, "service", "pkg", "sealitem.go"), `package pkg

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model/pkg"
)

type ItemSealer struct {
	service.Base[*pkg.Item, *pkg.Item, *pkg.Item]
}

func (s *ItemSealer) Create(ctx *gst.ServiceContext, item *pkg.Item) (*pkg.Item, error) {
	return item, nil
}
`)
	sealRecord := `package pkg

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model/pkg"
)

type RecordSealer struct {
	service.Base[*pkg.Record, *pkg.Record, *pkg.Record]
}

func (s *RecordSealer) Create(ctx *gst.ServiceContext, record *pkg.Record) (*pkg.Record, error) {
	ctx.Data(200, "text/plain", nil)
	return record, nil
}
`
	writeCheckFile(t, filepath.Join(projectDir, "service", "pkg", "sealrecord.go"), sealRecord)

	violations := runCheck(ggcheck.GRPCServiceContext)

	want := []string{fmt.Sprintf("service/pkg/sealrecord.go:%d: calls ctx.Data, which only HTTP serves; the model Record is served over gRPC as well, so keep its services to what both transports provide", sourceLine(t, sealRecord, "ctx.Data"))}
	if !slices.Equal(violations, want) {
		t.Fatalf("violations = %#v, want %#v", violations, want)
	}
}
