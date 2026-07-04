package rbac

import (
	"context"
	"maps"
	"strings"
	"sync"

	"github.com/casbin/casbin/v3"
	"github.com/cockroachdb/errors"
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
	enforcer   *casbin.ContextEnforcer
	enforcerMu sync.RWMutex
)

type rbac struct {
	enforcer *casbin.ContextEnforcer
	mu       *sync.RWMutex
}

// noop implements RBAC behavior before Casbin is initialized.
// It keeps the built-in root subject as system_root so modules that do not
// register authz can still use root-only administrative flows.
type noop struct{}

func (noop) Authorize(ctx context.Context, tenant string, subject string, object string, action string) (bool, error) {
	return false, nil
}

func (noop) AddRole(ctx context.Context, tenant string, role string) error    { return nil }
func (noop) RemoveRole(ctx context.Context, tenant string, role string) error { return nil }
func (noop) GrantPermission(ctx context.Context, tenant string, role string, object string, action string) error {
	return nil
}

func (noop) RevokePermission(ctx context.Context, tenant string, role string, object string, action string) error {
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

func RBAC() types.RBAC {
	// When RBAC is disabled or enforcer is not initialized,
	// return a safe no-op implementation to prevent panics.
	if enforcer == nil {
		return noop{}
	}
	return &rbac{
		enforcer: enforcer,
		mu:       &enforcerMu,
	}
}

// Authorize evaluates whether subject may perform action on object in tenant.
func (r *rbac) Authorize(ctx context.Context, tenant string, subject string, object string, action string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.enforcer.Enforce(tenant, subject, object, action)
}

// AddRole is a no-op because Casbin creates roles implicitly when a tenant
// receives permissions or grouping policies for that role.
func (r *rbac) AddRole(ctx context.Context, tenant string, role string) error {
	return nil
}

// RemoveRole removes all policies and subject assignments for role in tenant.
func (r *rbac) RemoveRole(ctx context.Context, tenant string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "remove_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	_, policyErr := r.enforcer.RemoveFilteredPolicyCtx(ctx, 0, tenant, role)
	_, groupingErr := r.enforcer.RemoveFilteredGroupingPolicyCtx(ctx, 1, role, tenant)
	err = errors.Join(policyErr, groupingErr)
	return err
}

// GrantPermission grants role access to object/action inside tenant.
func (r *rbac) GrantPermission(ctx context.Context, tenant string, role string, object string, action string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "grant_permission", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.AddPolicyCtx(ctx, tenant, role, object, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

// RevokePermission removes the exact tenant, role, object, action permission.
func (r *rbac) RevokePermission(ctx context.Context, tenant string, role string, object string, action string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "revoke_permission", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.RemovePolicyCtx(ctx, tenant, role, object, action, string(consts.EffectAllow)); err != nil {
		return err
	}
	return nil
}

// RevokeRolePermissions removes every permission policy granted to role in tenant.
// It is the explicit form of revoking a role's full permission set. Use
// RevokePermission when removing one concrete object/action grant.
func (r *rbac) RevokeRolePermissions(ctx context.Context, tenant string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "revoke_role_permissions", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.RemoveFilteredPolicyCtx(ctx, 0, tenant, role); err != nil {
		return err
	}
	return nil
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

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.AddGroupingPolicyCtx(ctx, subject, role, tenant); err != nil {
		return err
	}
	return nil
}

// UnassignRole removes a subject-role assignment from tenant.
func (r *rbac) UnassignRole(ctx context.Context, tenant string, subject string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "unassign_role", rbacTraceFields(tenant, role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.RemoveGroupingPolicyCtx(ctx, subject, role, tenant); err != nil {
		return err
	}
	return nil
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

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.AddNamedGroupingPolicyCtx(ctx, systemRoleGrouping, subject, role); err != nil {
		return err
	}
	return nil
}

// UnassignSystemRole removes a subject's system-level role assignment.
func (r *rbac) UnassignSystemRole(ctx context.Context, subject string, role string) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "unassign_system_role", rbacTraceFields("", role))
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err = r.enforcer.RemoveNamedGroupingPolicyCtx(ctx, systemRoleGrouping, subject, role); err != nil {
		return err
	}
	return nil
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
