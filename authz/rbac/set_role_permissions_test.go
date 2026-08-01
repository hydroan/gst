package rbac

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hydroan/gst/types"
)

// TestSetRolePermissionsReplacesTheWholeSet covers the replace semantics: the
// argument is the whole truth, so an entry the caller drops stops allowing
// requests and an empty set revokes everything.
func TestSetRolePermissionsReplacesTheWholeSet(t *testing.T) {
	r := newRolePermissionsFixture(t)
	ctx := context.Background()

	if err := r.SetRolePermissions(ctx, "default", "role_a", []types.Permission{
		{Object: "/api/things", Action: "GET"},
		{Object: "/api/things", Action: "POST"},
	}); err != nil {
		t.Fatal(err)
	}
	assertAuthorized(t, r, "/api/things", "GET", true)
	assertAuthorized(t, r, "/api/things", "POST", true)

	// Dropping POST from the set has to take that access away.
	if err := r.SetRolePermissions(ctx, "default", "role_a", []types.Permission{
		{Object: "/api/things", Action: "GET"},
	}); err != nil {
		t.Fatal(err)
	}
	assertAuthorized(t, r, "/api/things", "GET", true)
	assertAuthorized(t, r, "/api/things", "POST", false)

	if err := r.SetRolePermissions(ctx, "default", "role_a", nil); err != nil {
		t.Fatal(err)
	}
	assertAuthorized(t, r, "/api/things", "GET", false)
}

// TestSetRolePermissionsLeavesOtherRolesAlone guards the filter the replacement
// deletes by: it is scoped to one role in one tenant, so a role sharing either
// coordinate keeps its own permissions.
func TestSetRolePermissionsLeavesOtherRolesAlone(t *testing.T) {
	r := newRolePermissionsFixture(t)
	ctx := context.Background()

	for _, seed := range [][]string{
		{"default", "role_b", "/api/things", "GET", "allow"},
		{"other", "role_a", "/api/things", "GET", "allow"},
	} {
		if _, err := r.enforcer.AddPolicy(seed); err != nil {
			t.Fatal(err)
		}
	}

	if err := r.SetRolePermissions(ctx, "default", "role_a", nil); err != nil {
		t.Fatal(err)
	}

	for _, kept := range []struct {
		tenant string
		role   string
	}{
		{"default", "role_b"},
		{"other", "role_a"},
	} {
		policies, err := r.enforcer.GetFilteredPolicy(0, kept.tenant, kept.role)
		if err != nil {
			t.Fatal(err)
		}
		if len(policies) != 1 {
			t.Errorf("%s/%s: expected its permission to survive, got %v", kept.tenant, kept.role, policies)
		}
	}
}

// TestSetRolePermissionsDropsDuplicates pins the deduplication the batch insert
// depends on. Casbin's batch add does not deduplicate within a batch, so a
// caller repeating an entry would otherwise store the same policy twice.
func TestSetRolePermissionsDropsDuplicates(t *testing.T) {
	r := newRolePermissionsFixture(t)

	if err := r.SetRolePermissions(context.Background(), "default", "role_a", []types.Permission{
		{Object: "/api/things", Action: "GET"},
		{Object: "/api/things", Action: "GET"},
	}); err != nil {
		t.Fatal(err)
	}

	policies, err := r.enforcer.GetFilteredPolicy(0, "default", "role_a")
	if err != nil {
		t.Fatal(err)
	}
	if len(policies) != 1 {
		t.Errorf("expected one stored policy, got %v", policies)
	}
}

// TestSetRolePermissionsReplacesAtomically pins the property the method exists
// for: a concurrent authorization never observes the role between its old and
// its new permission set.
//
// Revoking the set and then granting it back one policy at a time releases the
// write lock between rows, which lets a reader in on an empty or partial set and
// denies a request the role is entitled to. Replacing under a single lock closes
// that window, so a reader sees either the whole old set or the whole new one.
func TestSetRolePermissionsReplacesAtomically(t *testing.T) {
	r := newRolePermissionsFixture(t)
	ctx := context.Background()

	permissions := make([]types.Permission, 0, 50)
	for i := range 50 {
		permissions = append(permissions, types.Permission{
			Object: fmt.Sprintf("/api/things/%d", i),
			Action: "GET",
		})
	}
	if err := r.SetRolePermissions(ctx, "default", "role_a", permissions); err != nil {
		t.Fatal(err)
	}

	var denied, failed atomic.Int64
	stop := make(chan struct{})
	var readers sync.WaitGroup
	readers.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			allowed, err := r.Authorize(ctx, "default", "u_member", "/api/things/0", "GET")
			if err != nil {
				failed.Add(1)
				return
			}
			if !allowed {
				denied.Add(1)
			}
		}
	})

	// Replacing with the same set still rewrites every row, so the window a
	// non-atomic replacement opens is exercised on each round.
	for range 20 {
		if err := r.SetRolePermissions(ctx, "default", "role_a", permissions); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	readers.Wait()

	if n := failed.Load(); n != 0 {
		t.Fatalf("authorization returned %d errors during replacement", n)
	}
	if n := denied.Load(); n != 0 {
		t.Errorf("expected no denial while the permission set is replaced, got %d", n)
	}
}

// TestSetPermissionsForAuthenticatedReplacesTheWholeSet covers the implicit
// authenticated role, which shares the replacement path with roles.
func TestSetPermissionsForAuthenticatedReplacesTheWholeSet(t *testing.T) {
	r := newRolePermissionsFixture(t)
	ctx := context.Background()

	if err := r.SetPermissionsForAuthenticated(ctx, map[string][]string{
		"/api/open": {"GET", "POST"},
	}); err != nil {
		t.Fatal(err)
	}
	// The authenticated role reaches subjects holding no role at all.
	for _, action := range []string{"GET", "POST"} {
		allowed, err := r.Authorize(ctx, "default", "u_plain", "/api/open", action)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Errorf("%s /api/open: expected the authenticated policy to allow it", action)
		}
	}

	if err := r.SetPermissionsForAuthenticated(ctx, nil); err != nil {
		t.Fatal(err)
	}
	allowed, err := r.Authorize(ctx, "default", "u_plain", "/api/open", "GET")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Error("expected an empty set to revoke every authenticated permission")
	}
}

// newRolePermissionsFixture builds an in-memory RBAC holding no policy, with
// u_member bound to role_a so permission changes are observable through
// Authorize.
func newRolePermissionsFixture(t *testing.T) *rbac {
	t.Helper()

	r := newTestRBAC(t, 0)
	if _, err := r.enforcer.AddGroupingPolicy("u_member", "role_a", "default"); err != nil {
		t.Fatal(err)
	}
	return r
}

func assertAuthorized(t *testing.T, r *rbac, object string, action string, want bool) {
	t.Helper()

	allowed, err := r.Authorize(context.Background(), "default", "u_member", object, action)
	if err != nil {
		t.Fatal(err)
	}
	if allowed != want {
		t.Errorf("%s %s: expected allowed=%v, got %v", action, object, want, allowed)
	}
}
