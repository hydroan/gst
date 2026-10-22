package modelschema_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// frameworkColumnsSample carries every framework column under the name the
// framework fixes: the fields of model.Base, and the tenant through
// tenant.Scope.
type frameworkColumnsSample struct {
	Name string

	modelregistry.Base
	tenant.Scope
}

// autoFrameworkColumnsSample carries them through model.AutoBase.
type autoFrameworkColumnsSample struct {
	Name string

	modelregistry.AutoBase
}

// redeclaredDeletedAtSample re-declares the soft-delete field under the
// column the framework fixes, which leaves the column where it is.
type redeclaredDeletedAtSample struct {
	DeletedAt gorm.DeletedAt `json:"-"`

	modelregistry.Base
}

// renamedFrameworkColumnsSample re-declares two framework fields under
// columns of its own.
type renamedFrameworkColumnsSample struct {
	CreatedBy string         `gorm:"column:author"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"column:removed_at"`

	modelregistry.Base
}

// prefixedTenantSample embeds tenant.Scope under a prefix, which renames the
// tenant column.
type prefixedTenantSample struct {
	modelregistry.Base
	tenant.Scope `gorm:"embeddedPrefix:owner_"`
}

// auditStamp holds fields under the Go names of framework fields, for a model
// to embed by name.
type auditStamp struct {
	CreatedAt time.Time
	CreatedBy string
}

// embeddedStampSample embeds auditStamp by name, so its fields are
// Audit.CreatedAt and Audit.CreatedBy rather than the framework's.
type embeddedStampSample struct {
	Audit auditStamp `gorm:"embedded;embeddedPrefix:audit_"`

	modelregistry.Base
}

// TestValidateFrameworkColumns pins which models keep the framework's columns
// under the names it fixes. model.Base, model.AutoBase and tenant.Scope as
// embedded pass, as does a framework field re-declared under its own column
// and a struct embedded by name whose fields only share the framework's Go
// names. A framework field re-declared under a column of the model's own, and
// tenant.Scope embedded under a prefix, are refused, every renamed column in
// one error with the fix.
func TestValidateFrameworkColumns(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"base_and_tenant":           reflect.TypeFor[frameworkColumnsSample](),
		"auto_base":                 reflect.TypeFor[*autoFrameworkColumnsSample](),
		"a_field_redeclared_as_is":  reflect.TypeFor[redeclaredDeletedAtSample](),
		"a_struct_embedded_by_name": reflect.TypeFor[embeddedStampSample](),
	} {
		t.Run("keeps_"+name, func(t *testing.T) {
			require.NoError(t, modelschema.ValidateFrameworkColumns(typ))
		})
	}

	t.Run("refuses_renamed_framework_fields", func(t *testing.T) {
		err := modelschema.ValidateFrameworkColumns(reflect.TypeFor[*renamedFrameworkColumnsSample]())
		require.EqualError(t, err, `model renamedFrameworkColumnsSample: framework field CreatedBy maps to column "author" instead of "created_by"; framework field DeletedAt maps to column "removed_at" instead of "deleted_at"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them`)
	})

	t.Run("refuses_a_prefixed_tenant", func(t *testing.T) {
		err := modelschema.ValidateFrameworkColumns(reflect.TypeFor[prefixedTenantSample]())
		require.EqualError(t, err, `model prefixedTenantSample: framework field TenantID maps to column "owner_tenant_id" instead of "tenant_id"; the framework fixes the names of its columns, so drop the column tag or embedded prefix renaming them`)
	})
}
