package rbac

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/persist"
	gstotel "github.com/hydroan/gst/provider/otel"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// | Operation                    | Casbin function                                           |
// | ---------------------------- | --------------------------------------------------------- |
// | Grant role permission        | `AddPolicy(tenant, role, obj, act, eft)`                  |
// | Revoke role permission       | `RemovePolicy(tenant, role, obj, act, eft)`               |
// | Revoke all role permissions  | `RemoveFilteredPolicy(0, tenant, role)`                   |
// | Assign role to subject       | `AddGroupingPolicy(subject, role, tenant)`                |
// | Unassign role from subject   | `RemoveGroupingPolicy(subject, role, tenant)`             |
// | Check subject tenant member  | `GetFilteredGroupingPolicy(0, subject)`                   |
// | Assign system role           | `AddNamedGroupingPolicy("g2", subject, role)`             |
// | Unassign system role         | `RemoveNamedGroupingPolicy("g2", subject, role)`          |
// | Query subject role in tenant | `GetFilteredGroupingPolicy(0, subject, role, tenant)`     |
// | Query role permissions       | `GetFilteredPolicy(0, tenant, role)`                      |
// | Query system role assignment | `HasNamedGroupingPolicy("g2", subject, role)`             |
// | Authorize request            | `Enforce(tenant, subject, obj, act)`                      |
//
// // Query subject role bindings in a tenant.
// RBAC.enforcer.GetFilteredGroupingPolicy(0, "user1", consts.AUTHZ_ROLE_ADMIN, DefaultTenant)
// // Query a subject's system-level role binding.
// RBAC.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, consts.AUTHZ_USER_ROOT, consts.AUTHZ_SYSTEM_ROLE_ROOT)
// // Query permissions granted to a role in a tenant.
// RBAC.enforcer.GetFilteredPolicy(0, DefaultTenant, "admin")
// // Authorize a subject against a tenant-scoped permission.
// RBAC.enforcer.Enforce(DefaultTenant, "user1", "/api/authz/routes", "GET")

// DefaultTenant is the built-in authorization domain used when no tenant
// resolver is configured by the application.
const DefaultTenant = "default"

const systemRoleGrouping = "g2"

var (
	enforcer    *casbin.ContextEnforcer
	policyStore *adapter
	enforcerMu  sync.RWMutex
)

type rbac struct {
	enforcer *casbin.ContextEnforcer

	// adapter is held directly rather than reached through the enforcer:
	// mutate drives the database half of a write on its own so it can defer the
	// in-memory half until the transaction commits.
	adapter persist.ContextBatchAdapter

	mu *sync.RWMutex
}

// noop implements RBAC behavior before Casbin is initialized.
// It keeps the built-in root subject as system_root so modules that do not
// register authz can still use root-only administrative flows.
type noop struct{}

func (noop) Authorize(ctx context.Context, tenant string, subject string, object string, action string) (bool, error) {
	return false, nil
}

func (noop) AuthorizeExplained(
	ctx context.Context, tenant string, subject string, object string, action string,
) (bool, consts.GrantSource, []string, error) {
	return false, "", nil, nil
}

func (noop) RemoveRole(ctx context.Context, tenant string, role string) error { return nil }
func (noop) GrantPermission(ctx context.Context, tenant string, role string, object string, action string) error {
	return nil
}

func (noop) RevokePermission(ctx context.Context, tenant string, role string, object string, action string) error {
	return nil
}

func (noop) SetRolePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) error {
	return nil
}

func (noop) SetPermissionsForAuthenticated(ctx context.Context, permissions []types.Permission) error {
	return nil
}

func (noop) RevokeRolePermissions(ctx context.Context, tenant string, role string) error {
	return nil
}

func (noop) AssignRole(ctx context.Context, tenant string, subject string, role string) error {
	return nil
}

func (noop) UnassignRole(ctx context.Context, tenant string, subject string, role string) error {
	return nil
}

func (noop) HasRole(ctx context.Context, tenant string, subject string, role string) (bool, error) {
	return false, nil
}

func (noop) SubjectInTenant(ctx context.Context, tenant string, subject string) (bool, error) {
	return false, nil
}

func (noop) SubjectsInTenant(ctx context.Context, tenant string) ([]string, error) { return nil, nil }

func (noop) AssignSystemRole(ctx context.Context, subject string, role string) error {
	return nil
}

func (noop) UnassignSystemRole(ctx context.Context, subject string, role string) error {
	return nil
}

func (noop) HasSystemRole(ctx context.Context, subject string, role string) (bool, error) {
	return isBuiltInSystemRole(subject, role), nil
}

func (noop) RemoveSubject(ctx context.Context, subject string) error {
	return nil
}

func (noop) ReloadPolicies(ctx context.Context) error { return nil }

func RBAC() types.RBAC {
	// When RBAC is disabled or enforcer is not initialized,
	// return a safe no-op implementation to prevent panics.
	if enforcer == nil {
		return noop{}
	}
	return &rbac{
		enforcer: enforcer,
		adapter:  policyStore,
		mu:       &enforcerMu,
	}
}

// Authorize evaluates whether subject may perform action on object in tenant.
func (r *rbac) Authorize(ctx context.Context, tenant string, subject string, object string, action string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enforcer.Enforce(tenant, subject, object, action)
}

// policyRoleColumn is the position of the role token in a p policy row. The
// policy definition is (tenant, role, obj, act, eft), so a row Casbin reports as
// the matched one carries its role here.
const policyRoleColumn = 1

// AuthorizeExplained evaluates the request and re-derives which matcher branch
// allowed it.
//
// The branches are checked in the order the matcher lists them, strongest
// first, and this order is load-bearing: a subject can satisfy several at once,
// and naming a weaker one would suggest that revoking it takes the access away.
// A system_root subject that also holds a role granting the same route must be
// reported as system_root, or an operator who then strips the role will find the
// route still reachable and no explanation for it.
//
// Keep this sequence aligned with the matcher in modelData. Nothing enforces the
// agreement: drift leaves the decisions correct and only the explanations wrong,
// which is the kind of fault that survives review.
//
// Only the two policy-driven branches yield a matched rule. The branches above
// them never consult a policy, so every stored row satisfies the matcher and
// Casbin reports whichever came first — an arbitrary row that would read as the
// reason for access.
func (r *rbac) AuthorizeExplained(
	ctx context.Context, tenant string, subject string, object string, action string,
) (bool, consts.GrantSource, []string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// The engine is called directly rather than through the exported helpers
	// below: those take the same read lock, and re-entering it deadlocks once a
	// writer is queued.
	allowed, matchedRule, err := r.enforcer.EnforceEx(tenant, subject, object, action)
	if err != nil || !allowed {
		return allowed, "", nil, err
	}

	systemRoot, err := r.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, subject, consts.AUTHZ_SYSTEM_ROLE_ROOT)
	if err != nil {
		return allowed, "", nil, err
	}
	if systemRoot {
		return allowed, consts.GrantSourceSystemRoot, nil, nil
	}

	tenantAdmin, err := r.enforcer.HasGroupingPolicy(subject, consts.AUTHZ_ROLE_ADMIN, tenant)
	if err != nil {
		return allowed, "", nil, err
	}
	if tenantAdmin {
		return allowed, consts.GrantSourceTenantAdmin, nil, nil
	}

	if len(matchedRule) > policyRoleColumn && matchedRule[policyRoleColumn] == consts.AUTHZ_ROLE_AUTHENTICATED {
		return allowed, consts.GrantSourceAuthenticated, matchedRule, nil
	}
	return allowed, consts.GrantSourceRole, matchedRule, nil
}

// RemoveRole removes all policies and subject assignments for role in tenant.
func (r *rbac) RemoveRole(ctx context.Context, tenant string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "remove_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(
		ctx,
		removeFiltered("p", "p", 0, tenant, role),
		removeFiltered("g", "g", 1, role, tenant),
	)
}

// GrantPermission grants role access to object/action inside tenant.
func (r *rbac) GrantPermission(ctx context.Context, tenant string, role string, object string, action string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "grant_permission", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, addRules("p", "p",
		[]string{tenant, role, object, action, string(consts.EffectAllow)}))
}

// RevokePermission removes the exact tenant, role, object, action permission.
func (r *rbac) RevokePermission(ctx context.Context, tenant string, role string, object string, action string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "revoke_permission", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeRules("p", "p",
		[]string{tenant, role, object, action, string(consts.EffectAllow)}))
}

// authenticatedPolicyTenant is the tenant column stored for policies written
// against consts.AUTHZ_ROLE_AUTHENTICATED. The matcher branch for that role
// compares no tenant, so the value never takes part in a decision; it marks the
// row as tenant-independent for anyone reading casbin_rule.
const authenticatedPolicyTenant = "*"

// SetRolePermissions replaces the entire permission set held by role in tenant.
func (r *rbac) SetRolePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "set_role_permissions", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.replacePermissions(ctx, tenant, role, permissions)
}

// SetPermissionsForAuthenticated replaces every permission held by all
// authenticated subjects with permissions. The policies are stored against the
// implicit authenticated role, which no grouping rule ever assigns, so they
// reach subjects holding no role at all.
//
// It shares its replacement with SetRolePermissions: the implicit role differs
// from a real one only in the tenant and role the policies are stored under, and
// keeping one implementation means the atomicity both depend on cannot hold for
// one of them and not the other.
func (r *rbac) SetPermissionsForAuthenticated(ctx context.Context, permissions []types.Permission) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "set_permissions_for_authenticated",
		rbacTraceFields(authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED))
	defer func() {
		finishSpan(err)
	}()

	return r.replacePermissions(ctx, authenticatedPolicyTenant, consts.AUTHZ_ROLE_AUTHENTICATED, permissions)
}

// replacePermissions swaps the whole permission set stored for role in tenant
// under a single write lock, so a concurrent Authorize sees either the old set
// or the new one and never the gap between them.
//
// The two engine calls form an ordering the correctness of the batch depends on:
// Casbin skips a batch insert entirely, and reports no error, when any rule in
// it already exists. Clearing the set first is what keeps that from silently
// dropping the whole replacement, so the delete must stay ahead of the insert.
func (r *rbac) replacePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) error {
	mutations := []policyMutation{removeFiltered("p", "p", 0, tenant, role)}
	if rules := permissionPolicies(tenant, role, permissions); len(rules) > 0 {
		mutations = append(mutations, addRules("p", "p", rules...))
	}
	return r.mutate(ctx, mutations...)
}

// permissionPolicies renders permissions as p policy rows for role in tenant.
//
// It drops repeats because Casbin's batch insert does not deduplicate within a
// batch, so a caller listing the same permission twice would otherwise store it
// twice. It sorts because the stored order decides which rule Casbin reports as
// the matched one: an order that followed the caller's would make
// AuthorizeExplained name a different rule for the same request whenever the
// caller derived its set from an unordered source.
func permissionPolicies(tenant string, role string, permissions []types.Permission) [][]string {
	unique := make([]types.Permission, 0, len(permissions))
	seen := make(map[types.Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		if permission.Object == "" || permission.Action == "" {
			continue
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		unique = append(unique, permission)
	}
	slices.SortFunc(unique, func(a, b types.Permission) int {
		if c := strings.Compare(a.Object, b.Object); c != 0 {
			return c
		}
		return strings.Compare(a.Action, b.Action)
	})

	rules := make([][]string, 0, len(unique))
	for _, permission := range unique {
		rules = append(rules, []string{
			tenant, role, permission.Object, permission.Action, string(consts.EffectAllow),
		})
	}
	return rules
}

// RevokeRolePermissions removes every permission policy granted to role in tenant.
// It is the explicit form of revoking a role's full permission set. Use
// RevokePermission when removing one concrete object/action grant.
func (r *rbac) RevokeRolePermissions(ctx context.Context, tenant string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "revoke_role_permissions", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeFiltered("p", "p", 0, tenant, role))
}

// AssignRole assigns subject to role inside tenant.
func (r *rbac) AssignRole(ctx context.Context, tenant string, subject string, role string) (err error) {
	if subject == role {
		return nil
	}
	ctx, finishSpan := traceRBAC(ctx, "assign_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, addRules("g", "g", []string{subject, role, tenant}))
}

// UnassignRole removes a subject-role assignment from tenant.
func (r *rbac) UnassignRole(ctx context.Context, tenant string, subject string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "unassign_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeRules("g", "g", []string{subject, role, tenant}))
}

// HasRole reports whether subject explicitly holds role inside tenant.
func (r *rbac) HasRole(ctx context.Context, tenant string, subject string, role string) (bool, error) {
	if subject == role {
		return false, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enforcer.HasGroupingPolicy(subject, role, tenant)
}

// SubjectInTenant reports whether subject has any role assignment inside tenant.
//
// Tenant membership is represented by Casbin grouping policies in the form
// subject, role, tenant. This check does not evaluate route permission; it only
// answers whether the subject belongs to the tenant authorization domain.
func (r *rbac) SubjectInTenant(ctx context.Context, tenant string, subject string) (bool, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return false, nil
	}
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = DefaultTenant
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	groupingPolicies, err := r.enforcer.GetFilteredGroupingPolicy(0, subject)
	if err != nil {
		return false, err
	}
	for _, policy := range groupingPolicies {
		if len(policy) >= 3 && strings.TrimSpace(policy[1]) != "" && policy[2] == tenant {
			return true, nil
		}
	}
	return false, nil
}

// SubjectsInTenant returns subjects with at least one role assignment inside tenant.
//
// It is used by IAM admin user list because IAM users do not store tenant_id.
// The tenant-visible user set is therefore derived from role bindings first and
// then joined back to user rows by subject ID.
func (r *rbac) SubjectsInTenant(ctx context.Context, tenant string) ([]string, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		tenant = DefaultTenant
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	groupingPolicies, err := r.enforcer.GetFilteredGroupingPolicy(2, tenant)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(groupingPolicies))
	subjects := make([]string, 0, len(groupingPolicies))
	for _, policy := range groupingPolicies {
		if len(policy) < 3 || strings.TrimSpace(policy[1]) == "" {
			continue
		}
		subject := strings.TrimSpace(policy[0])
		if subject == "" {
			continue
		}
		if _, ok := seen[subject]; ok {
			continue
		}
		seen[subject] = struct{}{}
		subjects = append(subjects, subject)
	}
	return subjects, nil
}

// AssignSystemRole assigns a subject to a system-level role outside any tenant.
func (r *rbac) AssignSystemRole(ctx context.Context, subject string, role string) (err error) {
	if subject == role {
		return nil
	}
	ctx, finishSpan := traceRBAC(ctx, "assign_system_role", rbacTraceFields("", role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, addRules("g", systemRoleGrouping, []string{subject, role}))
}

// UnassignSystemRole removes a subject's system-level role assignment.
func (r *rbac) UnassignSystemRole(ctx context.Context, subject string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "unassign_system_role", rbacTraceFields("", role))
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(ctx, removeRules("g", systemRoleGrouping, []string{subject, role}))
}

// HasSystemRole reports whether subject explicitly holds a system-level role.
func (r *rbac) HasSystemRole(ctx context.Context, subject string, role string) (bool, error) {
	if subject == role {
		return false, nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enforcer.HasNamedGroupingPolicy(systemRoleGrouping, subject, role)
}

// RemoveSubject removes every tenant-scoped and system-level role assignment
// held by subject, across all tenants.
func (r *rbac) RemoveSubject(ctx context.Context, subject string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "remove_subject", nil)
	defer func() {
		finishSpan(err)
	}()

	return r.mutate(
		ctx,
		removeFiltered("g", "g", 0, subject),
		removeFiltered("g", systemRoleGrouping, 0, subject),
	)
}

func contextOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

// traceRBAC starts a gst-owned RBAC span and returns a finish callback.
// The returned context must be passed to Casbin so adapter and database spans
// appear under the RBAC operation in the request trace.
func traceRBAC(ctx context.Context, operation string, fields map[string]any) (context.Context, func(error)) {
	ctx = contextOrBackground(ctx)
	if !gstotel.IsEnabled() {
		return ctx, func(error) {}
	}

	spanCtx, span := gstotel.StartSpan(ctx, gstotel.OperationSpanName("rbac", operation))
	recording := gstotel.IsSpanRecording(span)
	if recording {
		tags := map[string]any{
			"component":      "rbac",
			"rbac.operation": operation,
		}
		maps.Copy(tags, fields)
		gstotel.AddSpanTags(span, tags)
	}

	return spanCtx, func(err error) {
		defer span.End()
		if !recording {
			return
		}
		gstotel.AddSpanTags(span, map[string]any{
			"rbac.success": err == nil,
		})
		if err != nil {
			gstotel.RecordError(span, err)
		}
	}
}

// rbacTraceFields keeps RBAC span attributes low-cardinality enough for tracing.
// Subject identifiers are intentionally excluded because they are identity data
// and would make Jaeger labels noisy for role-binding write paths.
func rbacTraceFields(tenant string, role string) map[string]any {
	fields := make(map[string]any, 2)
	if tenant = strings.TrimSpace(tenant); tenant != "" {
		fields["rbac.tenant"] = tenant
	}
	if role = strings.TrimSpace(role); role != "" {
		fields["rbac.role"] = role
	}
	return fields
}

func isBuiltInSystemRole(subject string, role string) bool {
	return subject == consts.AUTHZ_USER_ROOT && role == consts.AUTHZ_SYSTEM_ROLE_ROOT
}
