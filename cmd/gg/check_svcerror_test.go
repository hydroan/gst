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
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"tmpapp/model"
)

type Getter struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (g *Getter) Get(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
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
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"tmpapp/model"
)

type Lister struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (l *Lister) List(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
	if err := loadRecords(ctx); err != nil {
		return nil, err
	}
	return &model.RecordRsp{}, nil
}

func loadRecords(ctx *types.ServiceContext) error {
	records := make([]*model.Record, 0)
	return database.Database[*model.Record](ctx).List(&records)
}
`)

	violations := CheckServiceErrorDiscipline(newProjectIgnoreMatcher())

	// Violations point at the raw error expressions themselves: the database
	// calls on laundry.go:23 / sample.go:19 / sample.go:26 and the raw
	// constructor on sample.go:23, since those are the places to wrap.
	wantSubstrings := []string{
		filepath.Join("service", "laundry", "laundry.go") + ":23:",
		filepath.Join("service", "sample", "sample.go") + ":19:",
		filepath.Join("service", "sample", "sample.go") + ":23:",
		filepath.Join("service", "sample", "sample.go") + ":26:",
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
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
	"tmpapp/helper/guard"
	"tmpapp/model"
)

type Updater struct {
	service.Base[*model.Record, *model.RecordReq, *model.RecordRsp]
}

func (u *Updater) Update(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
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

// loadChecked is a compliant generic helper: the instantiation wrapper must
// be transparent when the exit flow is resolved.
func loadChecked[T any](ctx *types.ServiceContext) error {
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
func (u *Updater) Patch(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
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
func (u *Updater) Delete(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
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
func (u *Updater) Import(ctx *types.ServiceContext, req *model.RecordReq) (*model.RecordRsp, error) {
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
	writeCheckFile(t, filepath.Join(projectDir, "helper", "guard", "guard.go"), `package guard

import (
	"net/http"

	"github.com/hydroan/gst/service"
	"github.com/hydroan/gst/types"
)

func RequireAdmin(ctx *types.ServiceContext) error {
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
