package service

import (
	"io"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/gst"
	"github.com/hydroan/gst/internal/model"
)

var _ gst.Service[*model.Empty, any, any] = (*Base[*model.Empty, any, any])(nil)

type Base[M gst.Model, REQ gst.Request, RSP gst.Response] struct{ gst.Logger }

func (Base[M, REQ, RSP]) Create(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Delete(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Update(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Patch(*gst.ServiceContext, REQ) (RSP, error)  { return *new(RSP), nil }
func (Base[M, REQ, RSP]) List(*gst.ServiceContext, REQ) (RSP, error)   { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Get(*gst.ServiceContext, REQ) (RSP, error)    { return *new(RSP), nil }

func (Base[M, REQ, RSP]) CreateMany(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) DeleteMany(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) UpdateMany(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) PatchMany(*gst.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) CreateBefore(*gst.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) CreateAfter(*gst.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) DeleteBefore(*gst.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) DeleteAfter(*gst.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) UpdateBefore(*gst.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) UpdateAfter(*gst.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) PatchBefore(*gst.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) PatchAfter(*gst.ServiceContext, M) error    { return nil }
func (Base[M, REQ, RSP]) ListBefore(*gst.ServiceContext, *[]M) error { return nil }
func (Base[M, REQ, RSP]) ListAfter(*gst.ServiceContext, *[]M) error  { return nil }
func (Base[M, REQ, RSP]) GetBefore(*gst.ServiceContext, M) error     { return nil }
func (Base[M, REQ, RSP]) GetAfter(*gst.ServiceContext, M) error      { return nil }

func (Base[M, REQ, RSP]) CreateManyBefore(*gst.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) CreateManyAfter(*gst.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) DeleteManyBefore(*gst.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) DeleteManyAfter(*gst.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) UpdateManyBefore(*gst.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) UpdateManyAfter(*gst.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) PatchManyBefore(*gst.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) PatchManyAfter(*gst.ServiceContext, ...M) error   { return nil }

// Import has no built-in parsing behavior: the DSL requires every Import
// action to declare Service(), so reaching this default means the route was
// wired without its service implementation. Answering loudly beats an
// empty-slice default, which would silently import nothing and mask the wiring
// error.
func (Base[M, REQ, RSP]) Import(*gst.ServiceContext, io.Reader) ([]M, error) {
	return nil, errors.New("import service is not implemented")
}

// Export mirrors Import: an empty-bytes default would export an empty file
// instead of surfacing the missing service.
func (Base[M, REQ, RSP]) Export(*gst.ServiceContext, ...M) ([]byte, error) {
	return nil, errors.New("export service is not implemented")
}

// SSE has no default streaming behavior: the DSL requires every SSE action to
// declare Service(), so reaching this default means the route was wired
// without its service implementation, which is answered loudly instead of
// with a silently empty stream.
func (Base[M, REQ, RSP]) SSE(*gst.ServiceContext) error {
	return errors.New("sse service is not implemented")
}

func (Base[M, REQ, RSP]) Filter(_ *gst.ServiceContext, m M, opts gst.QueryOptions) (M, gst.QueryOptions, error) {
	return m, opts, nil
}
