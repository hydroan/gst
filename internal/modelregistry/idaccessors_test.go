package modelregistry_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/stretchr/testify/require"
)

// shadowedIDSample and shadowedAutoIDSample declare an ID of their own over
// the base's while taking the base's ID accessors, which read and write the
// shadowed field; partialIDAccessorsSample declares GetID alone.
type shadowedIDSample struct {
	ID string `gorm:"primaryKey;size:191"`

	modelregistry.Base
}

type shadowedAutoIDSample struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`

	modelregistry.AutoBase
}

type partialIDAccessorsSample struct {
	ID string `gorm:"primaryKey;size:191"`

	modelregistry.Base
}

func (m *partialIDAccessorsSample) GetID() string { return m.ID }

// ownIDAccessorsSample declares an ID of its own and the accessors that go
// with it, the way a model widening its key does.
type ownIDAccessorsSample struct {
	ID string `gorm:"primaryKey;size:191"`

	modelregistry.Base
}

func (m *ownIDAccessorsSample) GetID() string { return m.ID }

func (m *ownIDAccessorsSample) SetID(id ...string) {
	if len(m.ID) == 0 && len(id) > 0 {
		m.ID = id[0]
	}
}

func (m *ownIDAccessorsSample) ClearID() { m.ID = "" }

// TestValidateIDAccessors pins which models read and write the ID they map:
// a model whose ID is its base's, or one declaring its own ID with its own
// GetID, SetID and ClearID, passes; one declaring its own ID while any of the
// three is still the base's is refused, the accessors it lacks named.
func TestValidateIDAccessors(t *testing.T) {
	t.Run("RefusesAShadowedIDWithoutItsOwnAccessors", func(t *testing.T) {
		require.EqualError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[*shadowedIDSample]()),
			"model shadowedIDSample: its ID shadows model.Base's; declare its own GetID, SetID and ClearID too: model.Base's read and write the shadowed field, which no column maps")
		require.EqualError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[shadowedAutoIDSample]()),
			"model shadowedAutoIDSample: its ID shadows model.AutoBase's; declare its own GetID, SetID and ClearID too: model.AutoBase's read and write the shadowed field, which no column maps")
	})

	t.Run("NamesTheAccessorsTheModelLacks", func(t *testing.T) {
		require.EqualError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[*partialIDAccessorsSample]()),
			"model partialIDAccessorsSample: its ID shadows model.Base's; declare its own SetID and ClearID too: model.Base's read and write the shadowed field, which no column maps")
	})

	t.Run("AcceptsAShadowedIDWithItsOwnAccessors", func(t *testing.T) {
		require.NoError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[*ownIDAccessorsSample]()))
	})

	t.Run("AcceptsTheBasesOwnID", func(t *testing.T) {
		require.NoError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[*BaseSample]()))
		require.NoError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[*AutoRecord]()))
		require.NoError(t, modelregistry.ValidateIDAccessors(reflect.TypeFor[string]()), "a type that is no struct declares no ID")
	})
}
