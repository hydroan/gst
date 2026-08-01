package rbac

import (
	"context"
	"fmt"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
)

// nullContextAdapter satisfies the adapter a ContextEnforcer requires while
// keeping the tests entirely in memory: policies are added through the enforcer
// and never persisted anywhere.
//
// It mirrors the capability surface of the adapter used at runtime, batch
// methods included. Casbin reaches the batch ones through a bare type
// assertion, so an adapter missing them panics rather than falling back.
type nullContextAdapter struct{}

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

	m, err := casbinmodel.NewModelFromString(string(modelData))
	if err != nil {
		tb.Fatal(err)
	}
	ctxEnforcer, err := casbin.NewContextEnforcer(m, new(nullContextAdapter))
	if err != nil {
		tb.Fatal(err)
	}
	e, ok := ctxEnforcer.(*casbin.ContextEnforcer)
	if !ok {
		tb.Fatal("expected a context enforcer")
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
	return &rbac{enforcer: newTestEnforcer(tb, policyCount), mu: &enforcerMu}
}
