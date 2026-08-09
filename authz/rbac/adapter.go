package rbac

import (
	"context"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// policyColumns are the rule columns of the policy table, in the order a rule
// fills them. ruleColumnCount is how many a rule can occupy.
var policyColumns = []string{"v0", "v1", "v2", "v3", "v4", "v5"}

const ruleColumnCount = 6

// policyRow is the row shape the adapter reads and writes.
//
// It describes how a rule maps onto columns, not what the table looks like:
// the schema belongs to the registered AuthzRule model, which is what creates
// the table and its unique index. Keep the column names in step with that model.
type policyRow struct {
	// ID is read-only so that it can name a row in an error without ever being
	// written: rules are inserted without one and the table assigns it.
	ID uint64 `gorm:"column:id;->"`

	Ptype string `gorm:"column:ptype"`
	V0    string `gorm:"column:v0"`
	V1    string `gorm:"column:v1"`
	V2    string `gorm:"column:v2"`
	V3    string `gorm:"column:v3"`
	V4    string `gorm:"column:v4"`
	V5    string `gorm:"column:v5"`

	// CreatedAt and UpdatedAt satisfy the base model's timestamp contract:
	// the columns are NOT NULL without a database default, so every insert
	// must carry both. gorm's naming convention fills them through the
	// connection's NowFunc — UTC in production. On a conflicting insert the
	// existing row keeps its values, which is what an idempotent re-add
	// should do.
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// adapter persists the policy set through the framework's database layer.
//
// It resolves its connection per operation from the context rather than
// capturing one at construction. A captured handle writes on its own
// connection, so a policy change made from inside a model hook would commit
// even when the transaction around that hook rolls back, leaving an
// authorization the record it came from no longer justifies.
//
// It never issues DDL. The table is created and migrated from the AuthzRule
// model like every other table.
type adapter struct {
	base  *gorm.DB
	table string
}

var _ policyStorage = (*adapter)(nil)

func newAdapter(base *gorm.DB, table string) *adapter {
	return &adapter{base: base, table: table}
}

// conn returns the connection this operation must run on: the transaction ctx
// carries when there is one, so the write joins it, and the plain handle
// otherwise.
func (a *adapter) conn(ctx context.Context) *gorm.DB {
	return dbruntime.Handle(ctx, a.base).WithContext(ctx).Table(a.table)
}

// loadPolicies reads every stored rule into a fresh policy set.
//
// It reads through the plain handle rather than the context transaction: it
// rebuilds the whole in-memory set, which must reflect committed state, not
// the half-finished state of a transaction still in flight.
func (a *adapter) loadPolicies(ctx context.Context) (*policySet, error) {
	rows := make([]policyRow, 0)
	if err := a.base.WithContext(ctx).Table(a.table).Order("id").Find(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "failed to load authz policies")
	}

	set := newPolicySet()
	for _, row := range rows {
		// A malformed row is not skipped: dropping it would silently change who
		// is authorized, which is worse than refusing to start. The row is named
		// by id so that whoever has to repair it can find it.
		rule, err := row.rule()
		if err != nil {
			return nil, err
		}
		set.add(row.Ptype, [][]string{rule})
	}
	return set, nil
}

// addPolicies stores rules, ignoring those already present.
//
// The conflict clause needs the unique index the AuthzRule model declares; it
// is what makes a repeated write idempotent instead of storing the rule twice.
func (a *adapter) addPolicies(ctx context.Context, ptype string, rules [][]string) error {
	if len(rules) == 0 {
		return nil
	}
	rows := make([]policyRow, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, newPolicyRow(ptype, rule))
	}
	err := a.conn(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	return errors.Wrap(err, "failed to add authz policies")
}

// removePoliciesCount deletes each rule by its exact column values and reports
// how many stored rows went.
//
// Every column is constrained, including the ones the rule leaves unused: the
// table stores those as empty strings, so an exact match is what distinguishes
// one rule from a longer rule sharing its prefix.
func (a *adapter) removePoliciesCount(ctx context.Context, ptype string, rules [][]string) (int64, error) {
	var removed int64
	for _, rule := range rules {
		conn := a.conn(ctx).Where("ptype = ?", ptype)
		values := newPolicyRow(ptype, rule).values()
		for i, column := range policyColumns {
			conn = conn.Where(column+" = ?", values[i])
		}
		if conn = conn.Delete(&policyRow{}); conn.Error != nil {
			return removed, errors.Wrap(conn.Error, "failed to remove authz policies")
		}
		removed += conn.RowsAffected
	}
	return removed, nil
}

// removeFilteredPolicyCount deletes the rules matching fieldValues starting at
// fieldIndex and reports how many stored rows went.
//
// An empty value matches any value in that column, which is the filter
// semantics the in-memory removal uses. A filter that is empty throughout
// therefore deletes every rule of this ptype, and the in-memory removal it is
// paired with does the same.
func (a *adapter) removeFilteredPolicyCount(
	ctx context.Context, ptype string, fieldIndex int, fieldValues ...string,
) (int64, error) {
	if fieldIndex < 0 || fieldIndex+len(fieldValues) > ruleColumnCount {
		return 0, errors.Newf("authz policy filter out of range: index %d, %d values", fieldIndex, len(fieldValues))
	}

	conn := a.conn(ctx).Where("ptype = ?", ptype)
	for i, value := range fieldValues {
		if value == "" {
			continue
		}
		conn = conn.Where(policyColumns[fieldIndex+i]+" = ?", value)
	}
	if conn = conn.Delete(&policyRow{}); conn.Error != nil {
		return 0, errors.Wrap(conn.Error, "failed to remove filtered authz policies")
	}
	return conn.RowsAffected, nil
}

// newPolicyRow spreads a rule across the value columns, leaving the unused ones
// empty so they match the table's defaults. A rule longer than the columns is
// truncated, which is the same limit the storage schema imposes.
func newPolicyRow(ptype string, rule []string) policyRow {
	var values [ruleColumnCount]string
	copy(values[:], rule)
	return policyRow{
		Ptype: ptype,
		V0:    values[0],
		V1:    values[1],
		V2:    values[2],
		V3:    values[3],
		V4:    values[4],
		V5:    values[5],
	}
}

func (r policyRow) values() [ruleColumnCount]string {
	return [ruleColumnCount]string{r.V0, r.V1, r.V2, r.V3, r.V4, r.V5}
}

// rule renders the row as the value slice the policy set holds, taking exactly
// as many columns as the row's kind declares.
//
// The count comes from ruleTokens and not from which columns happen to be
// non-empty. A rule is free to carry an empty token — an assignment in the
// default tenant reads naturally as one — and sizing the slice by the last
// non-empty column would hand the set a rule one value short of what its kind
// requires. What the row means has to be decided by what kind of row it is,
// never by its contents.
//
// A row whose ptype the table of kinds does not declare is refused rather than
// carried: the column is NOT NULL with an empty default and nothing constrains
// it to the declared kinds, so both an empty and an unknown ptype are reachable
// from the table alone, and a rule filed under either would decide nothing
// while looking stored.
func (r policyRow) rule() ([]string, error) {
	if r.Ptype == "" {
		return nil, errors.Newf("authz policy row %d has no ptype", r.ID)
	}
	tokens, ok := ruleTokens[r.Ptype]
	if !ok {
		return nil, errors.Newf("authz policy row %d has unknown ptype %q", r.ID, r.Ptype)
	}
	values := r.values()
	return values[:tokens], nil
}
