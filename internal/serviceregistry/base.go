package serviceregistry

import (
	"io"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
)

var _ types.Service[*modelregistry.Empty, any, any] = (*Base[*modelregistry.Empty, any, any])(nil)

type Base[M types.Model, REQ types.Request, RSP types.Response] struct{ types.Logger }

func (Base[M, REQ, RSP]) Create(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Delete(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Update(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Patch(*types.ServiceContext, REQ) (RSP, error)  { return *new(RSP), nil }
func (Base[M, REQ, RSP]) List(*types.ServiceContext, REQ) (RSP, error)   { return *new(RSP), nil }
func (Base[M, REQ, RSP]) Get(*types.ServiceContext, REQ) (RSP, error)    { return *new(RSP), nil }

func (Base[M, REQ, RSP]) CreateMany(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) DeleteMany(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) UpdateMany(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) PatchMany(*types.ServiceContext, REQ) (RSP, error) { return *new(RSP), nil }

func (Base[M, REQ, RSP]) CreateBefore(*types.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) CreateAfter(*types.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) DeleteBefore(*types.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) DeleteAfter(*types.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) UpdateBefore(*types.ServiceContext, M) error  { return nil }
func (Base[M, REQ, RSP]) UpdateAfter(*types.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) PatchBefore(*types.ServiceContext, M) error   { return nil }
func (Base[M, REQ, RSP]) PatchAfter(*types.ServiceContext, M) error    { return nil }
func (Base[M, REQ, RSP]) ListBefore(*types.ServiceContext, *[]M) error { return nil }
func (Base[M, REQ, RSP]) ListAfter(*types.ServiceContext, *[]M) error  { return nil }
func (Base[M, REQ, RSP]) GetBefore(*types.ServiceContext, M) error     { return nil }
func (Base[M, REQ, RSP]) GetAfter(*types.ServiceContext, M) error      { return nil }

func (Base[M, REQ, RSP]) CreateManyBefore(*types.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) CreateManyAfter(*types.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) DeleteManyBefore(*types.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) DeleteManyAfter(*types.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) UpdateManyBefore(*types.ServiceContext, ...M) error { return nil }
func (Base[M, REQ, RSP]) UpdateManyAfter(*types.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) PatchManyBefore(*types.ServiceContext, ...M) error  { return nil }
func (Base[M, REQ, RSP]) PatchManyAfter(*types.ServiceContext, ...M) error   { return nil }

// Import has no built-in parsing behavior: the DSL requires every Import
// action to declare Service(), so reaching this default means the route was
// wired without its service implementation. Answering loudly beats the old
// empty-slice default, which silently imported nothing and masked the wiring
// error.
func (Base[M, REQ, RSP]) Import(*types.ServiceContext, io.Reader) ([]M, error) {
	return nil, errors.New("import service is not implemented")
}

// Export mirrors Import: the old empty-bytes default exported an empty file
// instead of surfacing the missing service.
func (Base[M, REQ, RSP]) Export(*types.ServiceContext, ...M) ([]byte, error) {
	return nil, errors.New("export service is not implemented")
}

// SSE has no default streaming behavior: the DSL requires every SSE action to
// declare Service(), so reaching this default means the route was wired
// without its service implementation, which is answered loudly instead of
// with a silently empty stream.
func (Base[M, REQ, RSP]) SSE(*types.ServiceContext) error {
	return errors.New("sse service is not implemented")
}

func (Base[M, REQ, RSP]) Filter(_ *types.ServiceContext, m M, opts types.QueryOptions) (M, types.QueryOptions, error) {
	return m, opts, nil
}
