package controller

import (
	"context"
	"reflect"
	"slices"
	"unsafe"

	"github.com/hydroan/gst/internal/apiaction"
	"github.com/hydroan/gst/internal/execctx"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
	"github.com/hydroan/gst/internal/modelschema"
	modellogmgmt "github.com/hydroan/gst/internal/modulemodel/logmgmt"
	"github.com/hydroan/gst/internal/requestctx"
)

// This file holds what the CRUD flows share. A flow is the half of a handler
// that does not depend on the transport the request arrived on: the service
// hooks, the database access and the operation log, in the order the action
// defines. It runs on a context carrying the request metadata (see
// requestctx) and the controller span, and builds the service contexts of
// its hooks through a function the transport supplies, which is all a
// transport contributes to it. The HTTP handlers of this package bind the
// request, run the flow and write its answer; the gRPC calls do the same
// with the request message (see call_grpc.go). A flow's failure is the
// error of errors.go, which the transport renders.

// serviceContextFunc builds the service context of one hook or service call
// from the context of that call and its phase. Only the transport knows what
// else the service context should carry — the HTTP handler's gin context,
// nothing at all for a transport without one — so the transport supplies it.
type serviceContextFunc func(ctx context.Context, phase apiaction.Phase) *gst.ServiceContext

// writeFieldsOf reads off typ, the struct type of M, what the framework's
// own write flows go by: readOnly, the index paths of the fields M declares
// read-only (see patchFieldsOf); unwritten, the index paths of the columns
// an update leaves alone, those gorm writes on insert only or never (see
// modelschema.Column.Updatable) and the read-only ones, every column of a
// read-only field stored in several, a struct gorm embeds; and writable, the
// columns an update writes of a model declaring a read-only column, every
// column of the model's own but the unwritten ones. The columns of the
// framework's base are none of the model's own: the write names them itself,
// whatever columns it is narrowed to (see database.WithSelect). writable is
// nil for a model declaring no read-only column, whose update writes the
// full row, and empty for one declaring every column of its own read-only.
//
// For a model embedding model.Base beside Name, Stage `binding:"readonly"`
// and Tally, a struct of Seen and Passed tagged `gorm:"embedded"
// binding:"readonly"`, readOnly is the paths of Stage and Tally, unwritten
// the paths of the columns created_at, created_by, stage, seen and passed,
// and writable the column name.
func writeFieldsOf[M gst.Model](typ reflect.Type) (readOnly, unwritten [][]int, writable []gst.AnyColumnRef) {
	readOnly = patchFieldsOf(typ).readOnly
	columns, err := modelschema.Columns(typ)
	if err != nil {
		return readOnly, nil, nil
	}
	ofFrameworkBase := func(index []int) bool {
		holder := typ
		for _, i := range index[:len(index)-1] {
			step := holder.Field(i)
			holder = step.Type
			for holder.Kind() == reflect.Pointer {
				holder = holder.Elem()
			}
			if step.Anonymous && model.IsFrameworkBase(holder) {
				return true
			}
		}
		return false
	}
	narrowed := false
	own := make([]gst.AnyColumnRef, 0, len(columns))
	for _, col := range columns {
		// The path of a read-only field is the one its column is read by,
		// or, stored in several columns, the one each of them begins with.
		declared := slices.ContainsFunc(readOnly, func(index []int) bool {
			return len(index) <= len(col.Index) && slices.Equal(index, col.Index[:len(index)])
		})
		narrowed = narrowed || declared
		switch {
		case declared || !col.Updatable:
			unwritten = append(unwritten, col.Index)
		case !ofFrameworkBase(col.Index):
			own = append(own, gst.NewColumn[M, any](col.DBName))
		}
	}
	if narrowed {
		writable = own
	}
	return readOnly, unwritten, writable
}

// clearServerFields clears what a client sent in the fields of records the
// server writes alone: the creator and updater, and the fields the model
// declares read-only (see modelschema.IsReadOnly). Who created or updated a
// record is the write's to name, the caller the context carries as both (see
// database.Create), and with no caller, an anonymous request's, what a hook
// set, never what the client sent. The create and update flows call it
// first, before any hook runs, so a hook reads none of what the client sent
// there, and may set the fields on purpose: a create stores what it set in
// a read-only field, where an update writes no read-only column at all (see
// updateFlow).
func (a *action[M, REQ, RSP]) clearServerFields(records ...M) {
	for _, record := range records {
		record.SetCreatedBy("")
		record.SetUpdatedBy("")
		if len(a.readOnly) == 0 {
			continue
		}
		value := reflect.ValueOf(record).Elem()
		for _, index := range a.readOnly {
			// A field promoted through a nil embedded pointer holds nothing
			// to clear.
			if field, err := value.FieldByIndexErr(index); err == nil {
				field.SetZero()
			}
		}
	}
}

// operationLog returns the operation log entry of the request ctx carries,
// with the fields every action records filled in: who acted, from where, on
// which request, in which trace. The flow adds the record and the payloads.
func operationLog(ctx context.Context, modelName string) *modellogmgmt.OperationLog {
	meta := requestctx.FromContext(ctx)
	return &modellogmgmt.OperationLog{
		Model:     modelName,
		IP:        meta.ClientIP(),
		User:      meta.Username(),
		TraceID:   execctx.FromContext(ctx).TraceID,
		URI:       meta.RequestURI(),
		Method:    meta.Method(),
		UserAgent: meta.UserAgent(),
	}
}

// bytesToString converts a byte slice to a string without a memory
// allocation. For more details, see
// https://github.com/golang/go/issues/53003#issuecomment-1140276077.
func bytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// setID copies the id the request names a record by — the route parameter
// over HTTP, the id field of the message over gRPC — into the model and
// reports whether the model accepted it. UUID-keyed models (Base) accept any
// non-empty string, so the check never changes their behavior. Integer-keyed
// models (AutoBase) leave the id unset when the raw value does not parse
// into their key type; handlers must answer such requests with "not found"
// before any database access, because an unset id would silently drop the
// intended row filter and passing the raw value to SQL would rely on the
// database's implicit string-to-integer coercion (MySQL matches id=7 for
// '7abc').
//
// The caller must pass a non-empty id: UUID-keyed models generate a fresh id
// when given an empty value.
func setID(m gst.Model, id string) bool {
	m.SetID(id)
	return len(m.GetID()) > 0
}
