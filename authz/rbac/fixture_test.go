package rbac

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/hydroan/gst/internal/dbruntime"
	zaplogger "github.com/hydroan/gst/logger/zap"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestMain gives the package a database because every policy write opens a
// transaction. An in-memory SQLite one is enough: the tests that exercise only
// the in-memory model pair it with nullContextAdapter and never write a row, so
// all it has to do for them is begin and commit.
func TestMain(m *testing.M) {
	// Opening a transaction logs through logger.Database, and a failed in-memory
	// update logs through logger.Authz. Both are nil until the loggers are wired.
	if err := zaplogger.Init(); err != nil {
		panic(err)
	}
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{TranslateError: true})
	if err != nil {
		panic(err)
	}
	// Assigned rather than installed through dbruntime.InitDatabase, which also
	// starts the table builder. Tests needing a table create it themselves.
	dbruntime.DB = db
	os.Exit(m.Run())
}

// nullContextAdapter satisfies the adapter a ContextEnforcer requires while
// keeping the tests entirely in memory: policies are added through the enforcer
// and never persisted anywhere.
//
// It mirrors the capability surface of the adapter used at runtime, batch
// methods included. Casbin reaches the batch ones through a bare type
// assertion, so an adapter missing them panics rather than falling back.
//
// Its removal counts are unknown, not zero: a storage that keeps nothing
// cannot say how many rows a removal affected, and reporting zero would make
// every in-memory removal read as a disagreement to repair by reloading —
// through a loader that answers with an empty model.
type nullContextAdapter struct{}

func (*nullContextAdapter) removePoliciesCount(context.Context, string, [][]string) (int64, error) {
	return storedCountUnknown, nil
}

func (*nullContextAdapter) removeFilteredPolicyCount(
	context.Context, string, int, ...string,
) (int64, error) {
	return storedCountUnknown, nil
}

func (*nullContextAdapter) LoadPolicyCtx(context.Context, casbinmodel.Model) error { return nil }
func (*nullContextAdapter) SavePolicyCtx(context.Context, casbinmodel.Model) error { return nil }
func (*nullContextAdapter) AddPolicyCtx(context.Context, string, string, []string) error {
	return nil
}

func (*nullContextAdapter) RemovePolicyCtx(context.Context, string, string, []string) error {
	return nil
}

func (*nullContextAdapter) RemoveFilteredPolicyCtx(
	context.Context, string, string, int, ...string,
) error {
	return nil
}

func (*nullContextAdapter) AddPoliciesCtx(context.Context, string, string, [][]string) error {
	return nil
}

func (*nullContextAdapter) RemovePoliciesCtx(context.Context, string, string, [][]string) error {
	return nil
}

func (*nullContextAdapter) LoadPolicy(casbinmodel.Model) error          { return nil }
func (*nullContextAdapter) SavePolicy(casbinmodel.Model) error          { return nil }
func (*nullContextAdapter) AddPolicy(string, string, []string) error    { return nil }
func (*nullContextAdapter) RemovePolicy(string, string, []string) error { return nil }
func (*nullContextAdapter) RemoveFilteredPolicy(string, string, int, ...string) error {
	return nil
}
func (*nullContextAdapter) AddPolicies(string, string, [][]string) error    { return nil }
func (*nullContextAdapter) RemovePolicies(string, string, [][]string) error { return nil }

// newTestEnforcer builds an in-memory enforcer holding policyCount role
// policies. Benchmarks size it after a realistic deployment so the comparison
// reflects the per-policy matcher evaluation that dominates the cost; tests
// asking for none start from an empty policy set.
func newTestEnforcer(tb testing.TB, policyCount int) *casbin.ContextEnforcer {
	tb.Helper()

	e, err := newEnforcer(new(nullContextAdapter))
	if err != nil {
		tb.Fatal(err)
	}
	for i := range policyCount {
		if _, err = e.AddPolicy("default", "role_a", fmt.Sprintf("/api/things/%d", i), "GET", "allow"); err != nil {
			tb.Fatal(err)
		}
	}
	if _, err = e.AddGroupingPolicy("u1", "role_a", "default"); err != nil {
		tb.Fatal(err)
	}
	return e
}

func newTestRBAC(tb testing.TB, policyCount int) *rbac {
	tb.Helper()
	r := &rbac{
		enforcer: newTestEnforcer(tb, policyCount),
		adapter:  new(nullContextAdapter),
		mu:       &enforcerMu,
	}
	reindex(r)
	return r
}

// reindex rebuilds the decision index from r's model, standing in for the
// rebuild every real policy write performs. A fixture that seeds the model
// through the enforcer directly bypasses mutate, and a decision would
// otherwise answer from whatever index the previous test left installed.
func reindex(r *rbac) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rebuildPolicyIndex(r.enforcer.GetModel())
}

// dbruntimeDB is the package's test database handle.
func dbruntimeDB() *gorm.DB { return dbruntime.DB }

// newPolicyTable creates a policy table and returns an adapter bound to it, so
// that a test can exercise the real storage half instead of a null adapter.
//
// Each caller names its own table because the in-memory database is shared
// across the package.
func newPolicyTable(tb testing.TB, name string) *adapter {
	tb.Helper()

	ddl := "CREATE TABLE " + name + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ptype TEXT NOT NULL DEFAULT '',
		v0 TEXT NOT NULL DEFAULT '', v1 TEXT NOT NULL DEFAULT '', v2 TEXT NOT NULL DEFAULT '',
		v3 TEXT NOT NULL DEFAULT '', v4 TEXT NOT NULL DEFAULT '', v5 TEXT NOT NULL DEFAULT '',
		UNIQUE (ptype, v0, v1, v2, v3, v4, v5)
	)`
	if err := dbruntime.DB.Exec(ddl).Error; err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := dbruntime.DB.Exec("DROP TABLE " + name).Error; err != nil {
			tb.Error(err)
		}
	})
	return newAdapter(dbruntime.DB, name)
}

// storedRBAC pairs an enforcer with a real adapter over a real table, so a test
// can assert what a write left in storage as well as in memory.
//
// The enforcer is given the same adapter rather than a null one, so that
// reloading the model actually reads the table back.
func storedRBAC(tb testing.TB, table string) (*rbac, *adapter) {
	tb.Helper()
	store := newPolicyTable(tb, table)

	enforcer, err := newEnforcer(store)
	require.NoError(tb, err)

	r := &rbac{enforcer: enforcer, adapter: store, mu: &enforcerMu}
	reindex(r)
	return r, store
}

// memoryRules returns every rule the in-memory model holds, in the same shape
// storedRules reports, so the two can be compared directly.
func memoryRules(tb testing.TB, r *rbac) []string {
	tb.Helper()
	rules := make([]string, 0)
	for _, sec := range policySections {
		for ptype, ast := range r.enforcer.GetModel()[sec] {
			for _, rule := range ast.Policy {
				rules = append(rules, ptype+":"+strings.Join(rule, ","))
			}
		}
	}
	return rules
}

// storedRules returns every rule the table holds, as the loader sees it.
func storedRules(tb testing.TB, store *adapter) []string {
	tb.Helper()
	m, err := casbinmodel.NewModelFromString(string(modelData))
	require.NoError(tb, err)
	require.NoError(tb, store.LoadPolicyCtx(context.Background(), m))

	rules := make([]string, 0)
	for _, sec := range policySections {
		for ptype, ast := range m[sec] {
			for _, rule := range ast.Policy {
				rules = append(rules, ptype+":"+strings.Join(rule, ","))
			}
		}
	}
	return rules
}

// gaugeValue reads what a gauge currently holds, which is the only way to tell
// that the polarity of a published state is the right way round.
func gaugeValue(tb testing.TB, gauge prometheus.Gauge) float64 {
	tb.Helper()
	var metric dto.Metric
	require.NoError(tb, gauge.Write(&metric))
	return metric.GetGauge().GetValue()
}
