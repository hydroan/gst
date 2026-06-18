package serviceregistry

import (
	"io"

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

func (Base[M, REQ, RSP]) Import(*types.ServiceContext, io.Reader) ([]M, error) {
	return make([]M, 0), nil
}

func (Base[M, REQ, RSP]) Export(*types.ServiceContext, ...M) ([]byte, error) {
	return make([]byte, 0), nil
}

func (Base[M, REQ, RSP]) Filter(_ *types.ServiceContext, m M) M    { return m }
func (Base[M, REQ, RSP]) FilterRaw(_ *types.ServiceContext) string { return "" }
