package modelschema_test

import (
	"reflect"
	"testing"

	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/internal/modelschema"
	"github.com/hydroan/gst/internal/tenant"
	"github.com/stretchr/testify/require"
)

// tenantUniqueSample declares its uniqueness within the tenant, the tenant
// among the fields of the unique index.
type tenantUniqueSample struct {
	Code string `gorm:"size:64"`

	modelregistry.Base
	tenant.Scope
}

func (tenantUniqueSample) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"TenantID", "Code"}, Unique: true}}
}

// tenantGlobalUniqueSample declares a value unique across the whole table of
// a tenant-scoped model, twice over.
type tenantGlobalUniqueSample struct {
	Code string `gorm:"size:64"`
	Name string `gorm:"size:64"`

	modelregistry.Base
	tenant.Scope
}

func (tenantGlobalUniqueSample) Indexes() []modelschema.Index {
	return []modelschema.Index{
		{Fields: []string{"Code"}, Unique: true},
		{Fields: []string{"TenantID", "Name"}},
		{Fields: []string{"Name", "Code"}, Unique: true},
	}
}

// tenantPlainIndexSample declares an index across tenants that is not
// unique, which narrows nothing between tenants.
type tenantPlainIndexSample struct {
	Code string `gorm:"size:64"`

	modelregistry.Base
	tenant.Scope
}

func (tenantPlainIndexSample) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Code"}}}
}

// unscopedUniqueSample belongs to no tenant, so its uniqueness is the whole
// table's.
type unscopedUniqueSample struct {
	Code string `gorm:"size:64"`

	modelregistry.Base
}

func (unscopedUniqueSample) Indexes() []modelschema.Index {
	return []modelschema.Index{{Fields: []string{"Code"}, Unique: true}}
}

// TestValidateTenantIndexesHoldsUniquenessToTheTenant pins the rule and the
// error ValidateTenantIndexes documents: a unique index of a model embedding
// tenant.Scope names the tenant, every index leaving it out is reported in
// one error, and an index that is not unique, or a model without the tenant
// column, has nothing to report.
func TestValidateTenantIndexesHoldsUniquenessToTheTenant(t *testing.T) {
	for _, sample := range []any{&tenantUniqueSample{}, tenantPlainIndexSample{}, &unscopedUniqueSample{}} {
		require.NoError(t, modelschema.ValidateTenantIndexes(reflect.TypeOf(sample)), "%T", sample)
	}

	err := modelschema.ValidateTenantIndexes(reflect.TypeFor[*tenantGlobalUniqueSample]())
	require.EqualError(t, err, `model tenantGlobalUniqueSample: unique index (Code) leaves out TenantID; unique index (Name, Code) leaves out TenantID; a model embedding tenant.Scope is unique within a tenant, so every unique index names TenantID among its fields`)
}
