package dbruntime

import (
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/modelregistry"
	"github.com/hydroan/gst/types"
	"gorm.io/gorm"
)

// crossModelIndexPlans accumulates, per database handle, the index plans of
// every model whose indexes were ensured, so a later model mapping the same
// table is validated against them. Table creation consumes a stream that
// accepts registration at any stage, so there is no all-models synchronization
// point to validate at: the set grows one model at a time and revalidates in
// full on every arrival, which keeps the check independent of arrival order.
// Handles key the outer map because the same table name on two databases is
// two tables; the map is bounded by handles times model types and entries are
// never removed.
var (
	crossModelIndexPlansMu sync.Mutex
	crossModelIndexPlans   = make(map[*gorm.DB]map[reflect.Type]modelregistry.ModelIndexPlans)
)

// checkCrossModelIndexPlans records m's resolved plans and validates them
// against every other model already ensured on handler. Repeated arrivals of
// one model type are no conflict: the recorded plans are replaced.
func checkCrossModelIndexPlans(handler *gorm.DB, m types.Model, plans []modelregistry.IndexPlan) error {
	crossModelIndexPlansMu.Lock()
	defer crossModelIndexPlansMu.Unlock()

	byType := crossModelIndexPlans[handler]
	if byType == nil {
		byType = make(map[reflect.Type]modelregistry.ModelIndexPlans)
		crossModelIndexPlans[handler] = byType
	}
	byType[reflect.TypeOf(m)] = modelregistry.ModelIndexPlans{Model: modelDisplayName(m), Plans: plans}

	sets := make([]modelregistry.ModelIndexPlans, 0, len(byType))
	for _, set := range byType {
		sets = append(sets, set)
	}
	// Deterministic order keeps the reported conflict pair stable across runs.
	sort.Slice(sets, func(i, j int) bool { return sets[i].Model < sets[j].Model })
	return modelregistry.CheckCrossModelIndexPlanConflicts(sets)
}

// modelDisplayName renders the model's type name without pointer markers.
func modelDisplayName(m types.Model) string {
	typ := reflect.TypeOf(m)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ.String()
}

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
	tableName, err := requireTableName(m)
	if err != nil {
		return err
	}

	plans, err := modelregistry.ParseIndexPlans(handler, m)
	if err != nil || len(plans) == 0 {
		return err
	}
	if err = checkCrossModelIndexPlans(handler, m, plans); err != nil {
		return err
	}

	existing, err := handler.Migrator().GetIndexes(tableName)
	if err != nil {
		return errors.Wrapf(err, "failed to inspect indexes of table %q", tableName)
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
