package rbac

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

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
// the in-memory set pair it with nullStorage and never write a row, so all it
// has to do for them is begin and commit.
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

// nullStorage satisfies policyStorage while keeping a test entirely in
// memory: writes vanish, loads answer an empty set.
//
// Its removal counts are unknown, not zero: a storage that keeps nothing
// cannot say how many rows a removal affected, and reporting zero would make
// every in-memory removal read as a disagreement to repair — by reloading
// into an empty set.
type nullStorage struct{}

func (*nullStorage) loadPolicies(context.Context) (*policySet, error) { return newPolicySet(), nil }

func (*nullStorage) addPolicies(context.Context, string, [][]string) error { return nil }

func (*nullStorage) removePoliciesCount(context.Context, string, [][]string) (int64, error) {
	return storedCountUnknown, nil
}

func (*nullStorage) removeFilteredPolicyCount(
	context.Context, string, int, ...string,
) (int64, error) {
	return storedCountUnknown, nil
}

// installTestSet swaps the process's policy state for a fresh empty set and
// restores the uninstalled state when the test ends, so a test that never
// installs — the noop tests — is not handed a predecessor's policies.
func installTestSet(tb testing.TB) {
	tb.Helper()
	policyMu.Lock()
	policyRules = newPolicySet()
	rebuildDerived(policyRules)
	policyMu.Unlock()

	tb.Cleanup(func() {
		policyMu.Lock()
		policyRules = nil
		policyStore = nil
		rebuildDerived(nil)
		policyMu.Unlock()
	})
}

// seed writes rules straight into the installed policy set and rebuilds what
// is derived from it, standing in for the memory half every real write
// performs. Fixtures use it where they used to write through the enforcer.
func seed(tb testing.TB, ptype string, rules ...[]string) {
	tb.Helper()
	policyMu.Lock()
	defer policyMu.Unlock()
	policyRules.add(ptype, rules)
	rebuildDerived(policyRules)
}

// newEmptyRBAC builds an in-memory rbac over a freshly installed empty set,
// for tests that must account for every rule the process holds.
func newEmptyRBAC(tb testing.TB) *rbac {
	tb.Helper()
	installTestSet(tb)
	return &rbac{adapter: new(nullStorage), mu: &policyMu}
}

// newTestRBAC builds an in-memory rbac holding policyCount role policies and
// the baseline assignment granting them to u1.
// Benchmarks size it after a realistic deployment; tests asking for none start
// from an empty policy set.
func newTestRBAC(tb testing.TB, policyCount int) *rbac {
	tb.Helper()
	installTestSet(tb)
	for i := range policyCount {
		seed(tb, "p", []string{"default", "role_a", fmt.Sprintf("/api/things/%d", i), "GET", "allow"})
	}
	seed(tb, tenantRoleGrouping, []string{"u1", "role_a", "default"})
	return &rbac{adapter: new(nullStorage), mu: &policyMu}
}

// dbruntimeDB is the package's test database handle.
func dbruntimeDB() *gorm.DB { return dbruntime.DB }

// newPolicyTable creates a policy table and returns an adapter bound to it, so
// that a test can exercise the real storage half instead of a null one.
//
// Each caller names its own table because the in-memory database is shared
// across the package.
//
// The timestamp columns mirror the AuthzRule model, whose base declares them
// NOT NULL without a database default: a write that omits them must fail here
// the same way it would on the migrated production table.
func newPolicyTable(tb testing.TB, name string) *adapter {
	tb.Helper()

	ddl := "CREATE TABLE " + name + ` (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ptype TEXT NOT NULL DEFAULT '',
		v0 TEXT NOT NULL DEFAULT '', v1 TEXT NOT NULL DEFAULT '', v2 TEXT NOT NULL DEFAULT '',
		v3 TEXT NOT NULL DEFAULT '', v4 TEXT NOT NULL DEFAULT '', v5 TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
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

// storedRBAC pairs the installed policy state with a real adapter over a real
// table, so a test can assert what a write left in storage as well as in
// memory, and so that reloading actually reads the table back.
func storedRBAC(tb testing.TB, table string) (*rbac, *adapter) {
	tb.Helper()
	installTestSet(tb)
	store := newPolicyTable(tb, table)
	return &rbac{adapter: store, mu: &policyMu}, store
}

// memoryRules returns every rule the in-memory set holds, in the same shape
// storedRules reports, so the two can be compared directly.
func memoryRules(tb testing.TB, r *rbac) []string {
	tb.Helper()
	r.mu.RLock()
	defer r.mu.RUnlock()

	rules := make([]string, 0)
	for _, ptype := range rulePtypes {
		for _, rule := range policyRules.all(ptype) {
			rules = append(rules, ptype+":"+strings.Join(rule, ","))
		}
	}
	return rules
}

// storedRules returns every rule the table holds, as the loader sees it.
func storedRules(tb testing.TB, store *adapter) []string {
	tb.Helper()
	set, err := store.loadPolicies(context.Background())
	require.NoError(tb, err)

	rules := make([]string, 0)
	for _, ptype := range rulePtypes {
		for _, rule := range set.all(ptype) {
			rules = append(rules, ptype+":"+strings.Join(rule, ","))
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
