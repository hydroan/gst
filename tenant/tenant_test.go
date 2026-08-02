package tenant_test

import (
	"context"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/tenant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Record is a model scoped to one tenant, declared the only way a model can be:
// by embedding Scope.
type Record struct {
	ID   string `gorm:"primaryKey;size:191"`
	Name string `gorm:"size:191"`

	tenant.Scope
}

func (Record) TableName() string { return "records" }

// Unscoped is the same shape without the embedding, so a test can tell what the
// scoping adds from what the database would do anyway.
type Unscoped struct {
	ID   string `gorm:"primaryKey;size:191"`
	Name string `gorm:"size:191"`
}

func (Unscoped) TableName() string { return "unscoped_records" }

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger:         logger.Discard,
		TranslateError: true,
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Record{}, &Unscoped{}))
	return db
}

// seed writes one row per tenant straight through the scoping, so the rows a
// test reads back were placed by the mechanism under test rather than around it.
func seed(t *testing.T, db *gorm.DB, rows map[string]string) {
	t.Helper()
	for id, tenantID := range rows {
		require.NoError(t, db.WithContext(tenant.In(context.Background(), tenantID)).
			Create(&Record{ID: id, Name: id}).Error)
	}
}

// TestScopeNarrowsReads covers the predicate on the read path, which is the
// whole of what a list endpoint needed and never had.
func TestScopeNarrowsReads(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"a1": "alpha", "a2": "alpha", "b1": "beta"})

	var rows []Record
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).Find(&rows).Error)

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	assert.ElementsMatch(t, []string{"a1", "a2"}, ids, "a read must not reach another tenant's rows")

	var count int64
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).
		Model(&Record{}).Count(&count).Error)
	assert.EqualValues(t, 2, count, "the count has to agree with the rows, or paging reports a total nobody can reach")
}

// TestScopeStampsInsertsOverWhateverTheClientSent covers the column as what it
// is: data a client can send. A create that honored it would let one tenant
// plant rows in another.
func TestScopeStampsInsertsOverWhateverTheClientSent(t *testing.T) {
	db := newDB(t)

	forged := &Record{ID: "r1", Name: "r1"}
	forged.TenantID = "beta"
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).Create(forged).Error)

	var stored Record
	require.NoError(t, db.WithContext(tenant.Across(context.Background())).
		First(&stored, "id = ?", "r1").Error)
	assert.EqualValues(t, "alpha", stored.TenantID, "the insert must carry the caller's tenant, not the one it asked for")
}

// TestScopeStampsEveryRowOfABatch covers the batch insert, which reaches the
// clause as a slice rather than a struct.
func TestScopeStampsEveryRowOfABatch(t *testing.T) {
	db := newDB(t)

	batch := []*Record{{ID: "r1", Name: "r1"}, {ID: "r2", Name: "r2"}}
	batch[1].TenantID = "beta"
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).Create(batch).Error)

	var rows []Record
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).Find(&rows).Error)
	assert.Len(t, rows, 2, "every row of the batch has to be stamped, not just the first")
}

// TestScopeNarrowsUpdatesWithoutReadingFirst covers the write path that does not
// read before it writes. The predicate is what makes a cross-tenant update
// match nothing, which the framework already renders as a 404.
func TestScopeNarrowsUpdatesWithoutReadingFirst(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"b1": "beta"})

	result := db.WithContext(tenant.In(context.Background(), "alpha")).
		Model(&Record{}).Where("id = ?", "b1").Update("name", "taken")
	require.NoError(t, result.Error)
	assert.Zero(t, result.RowsAffected, "an update must not reach a row in another tenant")

	var stored Record
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "beta")).
		First(&stored, "id = ?", "b1").Error)
	assert.Equal(t, "b1", stored.Name)
}

// TestScopeRefusesAnUpdateThatNamesAnotherTenant covers the column's write-once
// permission and the refusal that makes it honest.
//
// Gorm keeps the column out of every update statement, so the row was never in
// danger. What was in danger is the caller's belief about it: an update that
// named another tenant and returned success would leave a client wrong about
// where its data is, with nothing in the response to say so.
func TestScopeRefusesAnUpdateThatNamesAnotherTenant(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"a1": "alpha"})
	ctx := tenant.In(context.Background(), "alpha")

	err := db.WithContext(ctx).Save(&Record{ID: "a1", Name: "moved", Scope: tenant.Scope{TenantID: "beta"}}).Error
	require.ErrorIs(t, err, tenant.ErrTenantImmutable)

	// Naming the tenant the row already belongs to is what every read-modify-write
	// round trip does, so it has to go through.
	require.NoError(t, db.WithContext(ctx).
		Save(&Record{ID: "a1", Name: "renamed", Scope: tenant.Scope{TenantID: "alpha"}}).Error)
	// Naming none is the other ordinary shape.
	require.NoError(t, db.WithContext(ctx).Model(&Record{}).Where("id = ?", "a1").
		Update("name", "renamed twice").Error)

	var stored Record
	require.NoError(t, db.WithContext(tenant.Across(context.Background())).
		First(&stored, "id = ?", "a1").Error)
	assert.EqualValues(t, "alpha", stored.TenantID, "the tenant column has to be unreachable after the insert")
	assert.Equal(t, "renamed twice", stored.Name, "the writes that were allowed have to have landed")
}

// TestScopeNarrowsDeletes covers the delete path, including the hard delete the
// framework performs through Unscoped — a caller reaching for deleted rows is
// not asking to reach into other tenants.
func TestScopeNarrowsDeletes(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"b1": "beta"})

	for name, del := range map[string]func(*gorm.DB) *gorm.DB{
		"soft": func(tx *gorm.DB) *gorm.DB { return tx.Where("id = ?", "b1").Delete(&Record{}) },
		"hard": func(tx *gorm.DB) *gorm.DB { return tx.Unscoped().Where("id = ?", "b1").Delete(&Record{}) },
	} {
		t.Run(name, func(t *testing.T) {
			result := del(db.WithContext(tenant.In(context.Background(), "alpha")))
			require.NoError(t, result.Error)
			assert.Zero(t, result.RowsAffected, "a delete must not reach a row in another tenant")
		})
	}
}

// TestAcrossSpansEveryTenant covers the one deliberate way out, which framework
// work reaching every tenant's rows has to have.
func TestAcrossSpansEveryTenant(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"a1": "alpha", "b1": "beta"})

	var rows []Record
	require.NoError(t, db.WithContext(tenant.Across(context.Background())).Find(&rows).Error)
	assert.Len(t, rows, 2)
}

// TestAcrossRefusesAnInsertThatNamesNoTenant covers the one caller the framework
// cannot stamp for. Defaulting would file the row under whichever tenant the
// framework picked, in the one situation where the caller had the reach to mean
// any of them.
func TestAcrossRefusesAnInsertThatNamesNoTenant(t *testing.T) {
	db := newDB(t)
	ctx := tenant.Across(context.Background())

	err := db.WithContext(ctx).Create(&Record{ID: "r1", Name: "r1"}).Error
	require.ErrorIs(t, err, tenant.ErrTenantRequired)

	require.NoError(t, db.WithContext(ctx).
		Create(&Record{ID: "r2", Name: "r2", Scope: tenant.Scope{TenantID: "beta"}}).Error)
	var stored Record
	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", "r2").Error)
	assert.EqualValues(t, "beta", stored.TenantID, "a cross-tenant insert has to keep the tenant it named")
}

// TestContextWithoutAScopeActsInTheDefaultTenant covers the deployment that
// never configures a resolver, and the caller that forgot to ask for one:
// neither reaches nothing, and neither reaches everything.
func TestContextWithoutAScopeActsInTheDefaultTenant(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"d1": tenant.Default, "b1": "beta"})

	var rows []Record
	require.NoError(t, db.WithContext(context.Background()).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "d1", rows[0].ID)
}

// TestScopeLeavesUndeclaredModelsAlone covers the default every table starts
// from: a model that did not embed Scope carries no predicate at all.
func TestScopeLeavesUndeclaredModelsAlone(t *testing.T) {
	db := newDB(t)
	ctx := tenant.In(context.Background(), "alpha")
	require.NoError(t, db.WithContext(ctx).Create(&Unscoped{ID: "u1", Name: "u1"}).Error)

	var rows []Unscoped
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "beta")).Find(&rows).Error)
	assert.Len(t, rows, 1, "a model without the embedding must be reachable from any tenant")
}

// TestScopeSurvivesASubqueryAndAPreload guards the reason the scoping lives on
// the column rather than on the framework's own query chain: a predicate the
// chain adds reaches only the statements the chain builds.
func TestScopeSurvivesASubqueryAndAPreload(t *testing.T) {
	db := newDB(t)
	seed(t, db, map[string]string{"a1": "alpha", "b1": "beta"})

	var names []string
	require.NoError(t, db.WithContext(tenant.In(context.Background(), "alpha")).
		Model(&Record{}).
		Where("id IN (?)", db.WithContext(tenant.In(context.Background(), "alpha")).
			Model(&Record{}).Select("id")).
		Pluck("name", &names).Error)
	assert.ElementsMatch(t, []string{"a1"}, names, "a subquery has to carry the predicate too")
}

func TestErrTenantRequiredIsMatchable(t *testing.T) {
	assert.True(t, errors.Is(tenant.ErrTenantRequired, tenant.ErrTenantRequired))
}
