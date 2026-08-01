package rbac

import (
	"context"

	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/internal/dbruntime"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// policyColumns are the rule columns of the policy table, in the order a Casbin
// rule fills them. ruleColumnCount is how many a rule can occupy.
var policyColumns = []string{"v0", "v1", "v2", "v3", "v4", "v5"}

const ruleColumnCount = 6

// policyRow is the row shape the adapter reads and writes.
//
// It describes how a rule maps onto columns, not what the table looks like:
// the schema belongs to the registered CasbinRule model, which is what creates
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
}

// adapter persists Casbin policies through the framework's database layer.
//
// It exists instead of the upstream GORM adapter for one reason: it resolves
// its connection per operation from the context rather than capturing one at
// construction. A captured handle writes on its own connection, so a policy
// change made from inside a model hook would commit even when the transaction
// around that hook rolls back, leaving an authorization the record it came from
// no longer justifies.
//
// It never issues DDL. The table is created and migrated from the CasbinRule
// model like every other table.
type adapter struct {
	base  *gorm.DB
	table string
}

var (
	_ persist.Adapter             = (*adapter)(nil)
	_ persist.ContextAdapter      = (*adapter)(nil)
	_ persist.BatchAdapter        = (*adapter)(nil)
	_ persist.ContextBatchAdapter = (*adapter)(nil)
)

func newAdapter(base *gorm.DB, table string) *adapter {
	return &adapter{base: base, table: table}
}

// conn returns the connection this operation must run on: the transaction ctx
// carries when there is one, so the write joins it, and the plain handle
// otherwise.
func (a *adapter) conn(ctx context.Context) *gorm.DB {
	return dbruntime.Handle(ctx, a.base).WithContext(ctx).Table(a.table)
}

// LoadPolicyCtx replaces m with every stored rule.
//
// It reads through the plain handle rather than the context transaction: it
// rebuilds the whole in-memory model, which must reflect committed state, not
// the half-finished state of a transaction still in flight.
func (a *adapter) LoadPolicyCtx(ctx context.Context, m model.Model) error {
	rows := make([]policyRow, 0)
	if err := a.base.WithContext(ctx).Table(a.table).Order("id").Find(&rows).Error; err != nil {
		return errors.Wrap(err, "failed to load casbin policies")
	}
	for _, row := range rows {
		// A malformed row is not skipped: dropping it would silently change who
		// is authorized, which is worse than refusing to start. The row is named
		// by id so that whoever has to repair it can find it.
		ast, err := row.assertion(m)
		if err != nil {
			return err
		}
		if err := persist.LoadPolicyArray(row.rule(ast), m); err != nil {
			return errors.Wrapf(err, "invalid casbin policy row %d", row.ID)
		}
	}
	return nil
}

// SavePolicyCtx replaces every stored rule with the contents of m.
func (a *adapter) SavePolicyCtx(ctx context.Context, m model.Model) error {
	rows := make([]policyRow, 0)
	for _, sec := range []string{"p", "g"} {
		for ptype, ast := range m[sec] {
			for _, rule := range ast.Policy {
				rows = append(rows, newPolicyRow(ptype, rule))
			}
		}
	}

	conn := a.conn(ctx)
	if err := conn.Where("1 = 1").Delete(&policyRow{}).Error; err != nil {
		return errors.Wrap(err, "failed to clear casbin policies")
	}
	if len(rows) == 0 {
		return nil
	}
	return errors.Wrap(a.conn(ctx).Create(&rows).Error, "failed to save casbin policies")
}

func (a *adapter) AddPolicyCtx(ctx context.Context, sec string, ptype string, rule []string) error {
	return a.AddPoliciesCtx(ctx, sec, ptype, [][]string{rule})
}

// AddPoliciesCtx stores rules, ignoring those already present.
//
// The conflict clause needs the unique index the CasbinRule model declares; it
// is what makes a repeated write idempotent instead of storing the rule twice.
func (a *adapter) AddPoliciesCtx(ctx context.Context, sec string, ptype string, rules [][]string) error {
	if len(rules) == 0 {
		return nil
	}
	rows := make([]policyRow, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, newPolicyRow(ptype, rule))
	}
	err := a.conn(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
	return errors.Wrap(err, "failed to add casbin policies")
}

func (a *adapter) RemovePolicyCtx(ctx context.Context, sec string, ptype string, rule []string) error {
	return a.RemovePoliciesCtx(ctx, sec, ptype, [][]string{rule})
}

// RemovePoliciesCtx deletes each rule by its exact column values.
//
// Every column is constrained, including the ones the rule leaves unused: the
// table stores those as empty strings, so an exact match is what distinguishes
// one rule from a longer rule sharing its prefix.
func (a *adapter) RemovePoliciesCtx(ctx context.Context, sec string, ptype string, rules [][]string) error {
	for _, rule := range rules {
		conn := a.conn(ctx).Where("ptype = ?", ptype)
		values := newPolicyRow(ptype, rule).values()
		for i, column := range policyColumns {
			conn = conn.Where(column+" = ?", values[i])
		}
		if err := conn.Delete(&policyRow{}).Error; err != nil {
			return errors.Wrap(err, "failed to remove casbin policies")
		}
	}
	return nil
}

// RemoveFilteredPolicyCtx deletes the rules matching fieldValues starting at
// fieldIndex.
//
// An empty value matches any value in that column, which is the filter
// semantics Casbin's in-memory removal uses. A filter that is empty throughout
// therefore deletes every rule of this ptype, and the in-memory removal it is
// paired with does the same.
func (a *adapter) RemoveFilteredPolicyCtx(
	ctx context.Context, sec string, ptype string, fieldIndex int, fieldValues ...string,
) error {
	if fieldIndex < 0 || fieldIndex+len(fieldValues) > ruleColumnCount {
		return errors.Newf("casbin filter out of range: index %d, %d values", fieldIndex, len(fieldValues))
	}

	conn := a.conn(ctx).Where("ptype = ?", ptype)
	for i, value := range fieldValues {
		if value == "" {
			continue
		}
		conn = conn.Where(policyColumns[fieldIndex+i]+" = ?", value)
	}
	return errors.Wrap(conn.Delete(&policyRow{}).Error, "failed to remove filtered casbin policies")
}

func (a *adapter) LoadPolicy(m model.Model) error { return a.LoadPolicyCtx(context.Background(), m) }

func (a *adapter) SavePolicy(m model.Model) error { return a.SavePolicyCtx(context.Background(), m) }

func (a *adapter) AddPolicy(sec string, ptype string, rule []string) error {
	return a.AddPolicyCtx(context.Background(), sec, ptype, rule)
}

func (a *adapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return a.RemovePolicyCtx(context.Background(), sec, ptype, rule)
}

func (a *adapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return a.RemoveFilteredPolicyCtx(context.Background(), sec, ptype, fieldIndex, fieldValues...)
}

func (a *adapter) AddPolicies(sec string, ptype string, rules [][]string) error {
	return a.AddPoliciesCtx(context.Background(), sec, ptype, rules)
}

func (a *adapter) RemovePolicies(sec string, ptype string, rules [][]string) error {
	return a.RemovePoliciesCtx(context.Background(), sec, ptype, rules)
}

// newPolicyRow spreads a rule across the value columns, leaving the unused ones
// empty so they match the table's defaults. A rule longer than the columns is
// truncated, which is the same limit Casbin's own storage schema imposes.
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

// rule renders the row as the token slice Casbin loads, taking exactly as many
// columns as the assertion it belongs to declares.
//
// The count comes from the assertion and not from which columns happen to be
// non-empty. A rule is free to carry an empty token — an assignment in the
// default tenant reads naturally as one — and sizing the slice by the last
// non-empty column would hand Casbin a rule one token short of what its
// assertion requires, which fails the load of the entire policy set. What the
// row means has to be decided by what kind of row it is, never by its contents.
func (r policyRow) rule(ast *model.Assertion) []string {
	values := r.values()
	return append([]string{r.Ptype}, values[:len(ast.Tokens)]...)
}

// assertion resolves the assertion a stored row belongs to.
//
// Casbin's loader derives the section from the first byte of the ptype and
// indexes the model with it, so it panics outright on an empty ptype and builds
// a rule nothing will ever match on an unknown one. Both are reachable from the
// table alone — the column is NOT NULL with an empty default, and nothing
// constrains it to the ptypes the model declares — so the row is resolved here
// first and reported as the load error the caller can act on.
func (r policyRow) assertion(m model.Model) (*model.Assertion, error) {
	if r.Ptype == "" {
		return nil, errors.Newf("casbin policy row %d has no ptype", r.ID)
	}
	ast, ok := m[r.Ptype[:1]][r.Ptype]
	if !ok {
		return nil, errors.Newf("casbin policy row %d has unknown ptype %q", r.ID, r.Ptype)
	}
	if len(ast.Tokens) > ruleColumnCount {
		return nil, errors.Newf(
			"casbin assertion %q declares %d tokens, more than the %d columns a rule is stored in",
			r.Ptype, len(ast.Tokens), ruleColumnCount,
		)
	}
	return ast, nil
}
