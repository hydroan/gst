package ggcheck_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hydroan/gst/internal/ggcheck"
	"github.com/hydroan/gst/internal/modelinspect"
)

// TestTenantUniqueIndexesRefusesAUniqueIndexLeavingOutTheTenant pins the
// check: a tenant-scoped model declaring a value unique across the whole
// table is reported under its file with the fix, before gg gen wrote any
// registration, and the same index declared within the tenant is clean.
func TestTenantUniqueIndexesRefusesAUniqueIndexLeavingOutTheTenant(t *testing.T) {
	projectDir := t.TempDir()
	t.Chdir(projectDir)
	// The check caches the column inspection under the user cache directory,
	// keyed by project directory; nothing reads a throwaway project's entry
	// again, so it goes when the test ends.
	cacheDir, err := modelinspect.CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(cacheDir); removeErr != nil {
			t.Error(removeErr)
		}
	})
	writeCheckProjectGoModAgainstRealFramework(t, projectDir)
	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), tenantScopedModel(`{Fields: []string{"Code"}, Unique: true}`))

	violations := runCheck(ggcheck.TenantUniqueIndexes)
	want := `model/record.go: model Record: unique index (Code) leaves out TenantID; a model embedding tenant.Scope is unique within a tenant, so every unique index names TenantID among its fields`
	if len(violations) != 1 || violations[0] != want {
		t.Fatalf("violations = %#v, want the unique index reported once under its file", violations)
	}

	writeCheckFile(t, filepath.Join(projectDir, "model", "record.go"), tenantScopedModel(`{Fields: []string{"TenantID", "Code"}, Unique: true}`))
	if violations := runCheck(ggcheck.TenantUniqueIndexes); len(violations) != 0 {
		t.Fatalf("uniqueness declared within the tenant should have nothing to report, got %#v", violations)
	}
}

// tenantScopedModel is a table-backed model embedding tenant.Scope that
// declares the one index given.
func tenantScopedModel(index string) string {
	return `package model

import (
	"github.com/hydroan/gst/dsl"
	"github.com/hydroan/gst/model"
	"github.com/hydroan/gst/tenant"
)

// Record is a record of one tenant.
type Record struct {
	Code string ` + "`json:\"code\" gorm:\"size:64\"`" + `

	model.Base
	tenant.Scope
}

func (Record) TableName() string { return "records" }

func (Record) Indexes() []model.Index { return []model.Index{` + index + `} }

func (Record) Design() {
	dsl.Migrate()
	dsl.Endpoint("records")
	dsl.Create(func() {})
}
`
}
