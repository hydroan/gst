package modelauthz

import (
	"context"
	"sync"
	"testing"

	"github.com/hydroan/gst/internal/requestctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

// TestMenuWriteGuardRequiresASystemSubject covers the boundary on menu writes.
// A menu is global and its routes are what every tenant's roles derive their
// permissions from, so an ordinary subject — even one whose policies grant it
// the route — must be refused, while the deployment's own writes, which carry
// no subject, must not be. The package holds no enforcer, so the guard answers
// from the noop implementation, which knows exactly the built-in root subject.
func TestMenuWriteGuardRequiresASystemSubject(t *testing.T) {
	withSubject := func(userID string) context.Context {
		return requestctx.WithMetadata(context.Background(),
			requestctx.New(requestctx.Fields{UserID: userID}))
	}

	t.Run("no request behind the write", func(t *testing.T) {
		require.NoError(t, errIfMenuWriteForbidden(context.Background()),
			"seeding and jobs carry no subject and are the deployment's own hand")
	})

	t.Run("system subject", func(t *testing.T) {
		require.NoError(t, errIfMenuWriteForbidden(withSubject("root")))
	})

	t.Run("ordinary subject", func(t *testing.T) {
		err := errIfMenuWriteForbidden(withSubject("member"))
		require.Error(t, err)
		require.ErrorContains(t, err, "system administrators")
	})
}

// TestMenuShadowedIDColumn asserts that the ID field declared on Menu, and not
// the shadowed model.Base field, is what GORM maps. Menu identifiers are stable
// hand-written keys, so the column has to be wide enough for them; a regression
// here surfaces only when a long identifier reaches the database and the driver
// rejects the whole batch.
func TestMenuShadowedIDColumn(t *testing.T) {
	parsed, err := schema.Parse(&Menu{}, &sync.Map{}, schema.NamingStrategy{})
	require.NoError(t, err)

	field := parsed.FieldsByDBName["id"]
	require.NotNil(t, field)
	require.Equal(t, "ID", field.Name)
	require.True(t, field.PrimaryKey)
	require.Equal(t, 191, field.Size)
	require.Equal(t, []string{"id"}, parsed.PrimaryFieldDBNames)
	require.Equal(t, 191, parsed.LookUpField("ParentID").Size,
		"parent_id stores the same values as id and must keep the same width")
}

// TestMenuIDAccessors asserts that the accessors address the shadowing field.
// model.Base declares them against the shadowed field, so without the overrides
// a menu would be persisted with an empty primary key.
func TestMenuIDAccessors(t *testing.T) {
	t.Run("keeps assigned id", func(t *testing.T) {
		m := &Menu{ID: "config/group"}
		m.SetID()
		require.Equal(t, "config/group", m.ID)
		require.Equal(t, "config/group", m.GetID())
		require.Empty(t, m.Base.ID, "the shadowed field must stay untouched")
	})

	t.Run("adopts requested id", func(t *testing.T) {
		m := new(Menu)
		m.SetID("query/sample_archived_record_history")
		require.Equal(t, "query/sample_archived_record_history", m.GetID())
	})

	t.Run("generates id when none is given", func(t *testing.T) {
		m := new(Menu)
		m.SetID()
		require.NotEmpty(t, m.GetID())

		other := new(Menu)
		other.SetID("")
		require.NotEmpty(t, other.GetID())
		require.NotEqual(t, m.GetID(), other.GetID())
	})

	t.Run("clears id", func(t *testing.T) {
		m := &Menu{ID: "sample/list"}
		m.ClearID()
		require.Empty(t, m.GetID())
	})
}
