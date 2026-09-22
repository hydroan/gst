package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckServiceErrorDisciplineFlagsRawErrorSources(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	// A service method returning raw framework errors, a raw cockroachdb
	// constructor, and a raw error inside a transaction closure.
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "sample.go"), `package sample

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Getter struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (g *Getter) Get(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	record := new(model.Record)
	if err := database.Database[*model.Record](ctx).Get(record, req.ID); err != nil {
		return nil, err
	}
	if req.ID == "" {
		return nil, errors.New("id is required")
	}
	err := database.Transaction(ctx, func(txCtx context.Context) error {
		if err := database.Database[*model.Record](txCtx).Update(record); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}
`)
	// A raw error laundered through a project helper still traces back to a
	// raw leaf inside the helper.
	writeCheckFile(t, filepath.Join(projectDir, "service", "laundry", "laundry.go"), `package laundry

import (
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Lister struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (l *Lister) List(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	if err := loadRecords(ctx); err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}

func loadRecords(ctx *gst.ServiceContext) error {
	records := make([]*model.Record, 0)
	return database.Database[*model.Record](ctx).List(&records)
}
`)

	// A name the method declares itself shadows the package-level meaning of
	// the same name: mgr is the local other, not the package's manager, and
	// service is a local other, not the framework package.
	writeCheckFile(t, filepath.Join(projectDir, "service", "shadow", "shadow.go"), `package shadow

import (
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Getter struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

type other struct{}

func (other) Do() error { return errors.New("raw") }

func (other) NewError() error { return errors.New("raw constructor") }

type manager struct{}

func (manager) Do() error { return service.NewError(http.StatusBadRequest, "bad request") }

var mgr = manager{}

func (g *Getter) Get(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	mgr := other{}
	return nil, mgr.Do()
}

func (g *Getter) Delete(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	service := other{}
	return nil, service.NewError()
}

// List calls Do on a local variable whose declaration does not spell its
// type out, so the call fails closed.
func (g *Getter) List(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	cli := newOther()
	return nil, cli.Do()
}

func newOther() other { return other{} }
`)

	violations := CheckServiceErrorDiscipline(newProjectIgnoreMatcher())

	// Violations point at the raw error expressions themselves: the database
	// calls on laundry.go:23 / sample.go:19 / sample.go:26, the raw
	// constructor on sample.go:23, the raw constructors the shadowing locals
	// reach on shadow.go:18 and shadow.go:20, and the call on a local of
	// unknown type on shadow.go:42, since those are the places to wrap.
	wantSubstrings := []string{
		filepath.Join("service", "laundry", "laundry.go") + ":23:",
		filepath.Join("service", "sample", "sample.go") + ":19:",
		filepath.Join("service", "sample", "sample.go") + ":23:",
		filepath.Join("service", "sample", "sample.go") + ":26:",
		filepath.Join("service", "shadow", "shadow.go") + ":18:",
		filepath.Join("service", "shadow", "shadow.go") + ":20:",
		filepath.Join("service", "shadow", "shadow.go") + ":42:",
	}
	if len(violations) != len(wantSubstrings) {
		t.Fatalf("expected %d violations, got %#v", len(wantSubstrings), violations)
	}
	for i, want := range wantSubstrings {
		if !strings.Contains(violations[i], want) {
			t.Fatalf("violation %d should contain %q, got %q", i, want, violations[i])
		}
	}
	for _, v := range violations {
		if !strings.Contains(v, "service.NewError") {
			t.Fatalf("violation should point at service.NewError as the fix, got %q", v)
		}
	}
}

func TestCheckServiceErrorDisciplineAllowsCompliantSources(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)

	writeCheckFile(t, filepath.Join(projectDir, "go.mod"), "module tmpapp\n\ngo 1.26\n")
	// Every service exit is nil, a NewError construction, a compliant helper
	// in another project package, a compliant receiver method, or a
	// transaction closure whose exits are compliant.
	writeCheckFile(t, filepath.Join(projectDir, "service", "sample", "sample.go"), `package sample

import (
	"context"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/sse"
	"tmpapp/helper/guard"
	"tmpapp/model"
)

type Updater struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (u *Updater) Update(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	if err := guard.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := loadChecked[*model.Record](ctx); err != nil {
		return nil, err
	}
	if err := u.validate(req); err != nil {
		return nil, err
	}
	record := new(model.Record)
	if err := database.Database[*model.Record](ctx).Get(record, req.ID); err != nil {
		return nil, newRecordMissingError(err)
	}
	err := database.Transaction(ctx, func(txCtx context.Context) error {
		if err := database.Database[*model.Record](txCtx).Update(record); err != nil {
			return service.NewErrorWithCause(http.StatusConflict, "record update failed", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}

func (u *Updater) validate(req *model.RecordReq) error {
	if req.ID == "" {
		return service.NewError(http.StatusBadRequest, "id is required")
	}
	return nil
}

// SSE returns the framework streaming call directly: ServiceContext.SSE
// errors are framework-governed and count as a sanctioned exit.
func (u *Updater) SSE(ctx *gst.ServiceContext) error {
	return ctx.SSE(func(conn *sse.Conn) error {
		return conn.Send(sse.Event{Data: "sample"})
	})
}

// loadChecked is a compliant generic helper: the instantiation wrapper must
// be transparent when the exit flow is resolved.
func loadChecked[T any](ctx *gst.ServiceContext) error {
	if ctx == nil {
		return service.NewError(http.StatusBadRequest, "context is required")
	}
	return nil
}

// newRecordMissingError returns *service.Error, which is compliant by
// construction and needs no body analysis.
func newRecordMissingError(err error) *service.Error {
	return service.NewErrorWithCause(http.StatusNotFound, "record not found", err)
}

// Patch reuses one err variable for several sources. The early compliant
// return must not be polluted by the raw assignment that happens later in
// the body: only assignments before a return feed that return.
func (u *Updater) Patch(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	err := guard.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	record := new(model.Record)
	err = database.Database[*model.Record](ctx).Get(record, req.ID)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusNotFound, "record not found", err)
	}
	return &model.RecordRsp{}, nil
}

// Delete reuses one err variable the idiomatic way. Every raw assignment is
// checked and answered right away, which kills it for everything after the
// check, so the later compliant flows through the same variable stay clean.
func (u *Updater) Delete(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	record := new(model.Record)
	err := database.Database[*model.Record](ctx).Get(record, req.ID)
	if err != nil {
		return nil, service.NewErrorWithCause(http.StatusNotFound, "record not found", err)
	}
	err = guard.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}
	if err = database.Database[*model.Record](ctx).Delete(record); err != nil {
		return nil, service.NewErrorWithCause(http.StatusInternalServerError, "record delete failed", err)
	}
	if err = guard.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}

// Import mirrors a load-or-default flow: the not-found branch of the first
// check falls through instead of returning, so that raw assignment is never
// killed; the next check still stays clean because inside a check's body the
// variable holds only the value its own init just assigned.
func (u *Updater) Import(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	record := new(model.Record)
	err := database.Database[*model.Record](ctx).Get(record, req.ID)
	if err != nil {
		if !errors.Is(err, database.ErrRecordNotFound) {
			return nil, service.NewErrorWithCause(http.StatusInternalServerError, "failed to load record", err)
		}
		record = &model.Record{}
	}
	if err = guard.RequireAdmin(ctx); err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}
`)
	// Methods called on a package-level singleton, on local variables and on
	// a parameter whose declarations spell their type out resolve to that
	// type's methods.
	writeCheckFile(t, filepath.Join(projectDir, "service", "singleton", "singleton.go"), `package singleton

import (
	"net/http"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
	"tmpapp/model"
)

type Getter struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

type manager struct{}

func (manager) Do() error { return service.NewError(http.StatusBadRequest, "bad request") }

var mgr = manager{}

func (g *Getter) Get(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	return nil, mgr.Do()
}

func (g *Getter) List(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	local := manager{}
	if err := local.Do(); err != nil {
		return nil, err
	}
	var declared manager
	if err := declared.Do(); err != nil {
		return nil, err
	}
	pointer := &manager{}
	if err := pointer.Do(); err != nil {
		return nil, err
	}
	return nil, run(manager{})
}

func run(m manager) error { return m.Do() }

// Update calls a compliant helper that calls itself.
func (g *Getter) Update(ctx *gst.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	return nil, countdown(3)
}

func countdown(n int) error {
	if n == 0 {
		return service.NewError(http.StatusBadRequest, "countdown finished")
	}
	return countdown(n - 1)
}
`)
	writeCheckFile(t, filepath.Join(projectDir, "helper", "guard", "guard.go"), `package guard

import (
	"net/http"

	"github.com/hydroan/gst"
	"github.com/hydroan/gst/service"
)

func RequireAdmin(ctx *gst.ServiceContext) error {
	if ctx == nil {
		return service.NewError(http.StatusForbidden, "admin required")
	}
	return nil
}
`)
	// Functions outside service structs are not entry points; their raw
	// returns stay unreported as long as no service exit reaches them.
	writeCheckFile(t, filepath.Join(projectDir, "cronjob", "job.go"), `package cronjob

import (
	"github.com/hydroan/gst/database"
	"tmpapp/model"
)

func Sweep() error {
	records := make([]*model.Record, 0)
	return database.Database[*model.Record](nil).List(&records)
}
`)

	violations := CheckServiceErrorDiscipline(newProjectIgnoreMatcher())
	if len(violations) != 0 {
		t.Fatalf("expected no violations, got %#v", violations)
	}
}
