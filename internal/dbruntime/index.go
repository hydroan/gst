package dbruntime

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

// ensureCustomIndexes validates the model's custom index declarations and
// creates the missing ones right after AutoMigrate, mirroring how struct tag
// indexes are ensured at bootstrap. It never drops or alters existing
// indexes: a plan name occupied by a different definition and a
// same-definition index under a different name both return an error so
// schema drift fails bootstrap instead of being repaired silently.
//
// Index renames therefore follow the migrate-then-deploy workflow: apply the
// rename through gg migrate first, so this bootstrap path finds the new name
// already in place and does nothing.
func ensureCustomIndexes(handler *gorm.DB, m types.Model) error {
	plans, err := modelregistry.ParseIndexPlans(handler, m, m.GetTableName())
	if err != nil || len(plans) == 0 {
		return err
	}

	existing, err := handler.Migrator().GetIndexes(m.GetTableName())
	if err != nil {
		return errors.Wrapf(err, "failed to inspect indexes of table %q", m.GetTableName())
	}
	byName := make(map[string]gorm.Index, len(existing))
	for _, idx := range existing {
		byName[idx.Name()] = idx
	}

	for _, plan := range plans {
		if idx, ok := byName[plan.Name]; ok {
			if !matchesPlan(idx, plan) {
				return errors.Newf("index %q on table %q exists with a different definition; resolve the conflict manually",
					plan.Name, plan.Table)
			}
			continue
		}
		if renamed := sameDefinitionName(existing, plan); len(renamed) != 0 {
			return errors.Newf("index on table %q columns (%s) already exists as %q; rename it manually, e.g. ALTER TABLE %s RENAME INDEX %s TO %s",
				plan.Table, strings.Join(plan.Columns, ","), renamed, plan.Table, renamed, plan.Name)
		}
		if err = handler.Exec(plan.CreateSQL(handler.Dialector)).Error; err != nil {
			return errors.Wrapf(err, "failed to create index %q on table %q", plan.Name, plan.Table)
		}
	}
	return nil
}

// matchesPlan reports whether an existing index carries the plan's column
// sequence and uniqueness. Uniqueness participates only when the driver
// reports it.
func matchesPlan(idx gorm.Index, plan modelregistry.IndexPlan) bool {
	columns := idx.Columns()
	if len(columns) != len(plan.Columns) {
		return false
	}
	for i, col := range columns {
		if col != plan.Columns[i] {
			return false
		}
	}
	if unique, ok := idx.Unique(); ok && unique != plan.Unique {
		return false
	}
	return true
}

// sameDefinitionName returns the name of an existing non-primary index that
// matches the plan's definition, signaling a rename candidate.
func sameDefinitionName(existing []gorm.Index, plan modelregistry.IndexPlan) string {
	for _, idx := range existing {
		if isPrimary, ok := idx.PrimaryKey(); ok && isPrimary {
			continue
		}
		if matchesPlan(idx, plan) {
			return idx.Name()
		}
	}
	return ""
}
