package rbac

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
)

// ErrRBACDisabled reports a policy write made in a process that holds no policy
// set to write it to.
//
// A write has nowhere to go before Init has installed a policy set: there is no
// in-memory set to change and no adapter to store the change through. Reporting
// success would tell the caller its change is in force — a role created through
// the API answering success with not one policy row behind it — and nothing
// downstream can tell that apart from a change that landed, because the records
// the rules are derived from were written either way.
//
// Reads are answered instead of refused. A denial and an empty role set are both
// true of a process deciding from no policies, so answering them states the
// situation rather than hiding it.
var ErrRBACDisabled = errors.New("rbac: authorization is not initialized")

// noop implements RBAC behavior before a policy set is installed.
// It answers reads and refuses writes with ErrRBACDisabled, and keeps the
// built-in root subject as system_root so modules that do not register authz can
// still use root-only administrative flows.
type noop struct{}

// Authorize denies, and says that it is the absence of a policy set doing it
// rather than the absence of a rule. Every request in the process is refused
// the same way, which reads in the log as a deployment-wide misconfiguration
// instead of a permission somebody forgot to grant.
func (noop) Authorize(
	ctx context.Context, tenant string, subject string, object string, action string,
) (types.Decision, error) {
	return types.Decision{Reason: consts.DenyReasonNotInitialized}, nil
}

func (noop) RemoveRole(ctx context.Context, tenant string, role string) error {
	return ErrRBACDisabled
}

func (noop) SetRolePermissions(
	ctx context.Context, tenant string, role string, permissions []types.Permission,
) error {
	return ErrRBACDisabled
}

func (noop) SetPermissionsForAuthenticated(ctx context.Context, permissions []types.Permission) error {
	return ErrRBACDisabled
}

func (noop) AssignRole(ctx context.Context, tenant string, subject string, role string) error {
	return ErrRBACDisabled
}

func (noop) UnassignRole(ctx context.Context, tenant string, subject string, role string) error {
	return ErrRBACDisabled
}

func (noop) RolesForSubject(ctx context.Context, tenant string, subject string) ([]string, error) {
	return nil, nil
}

func (noop) SubjectsInTenant(ctx context.Context, tenant string) ([]string, error) { return nil, nil }

func (noop) AssignSystemRole(ctx context.Context, subject string, role string) error {
	return ErrRBACDisabled
}

func (noop) UnassignSystemRole(ctx context.Context, subject string, role string) error {
	return ErrRBACDisabled
}

func (noop) HasSystemRole(ctx context.Context, subject string, role string) (bool, error) {
	return isBuiltInSystemRole(subject, role), nil
}

func (noop) RemoveSubject(ctx context.Context, subject string) error {
	return ErrRBACDisabled
}

// ReloadPolicies succeeds because there is no in-memory policy set to rebuild.
// Nothing is claimed by saying so: the caller asked this process to catch up
// with storage, and a process deciding from no policies already has.
func (noop) ReloadPolicies(ctx context.Context) error { return nil }
