package rbac

import (
	"context"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
)

// policyOp is what a mutation does to the stored policies.
type policyOp int

const (
	policyAdd policyOp = iota
	policyRemove
	policyRemoveFiltered
)

// policyMutation is one change to the policy store, expressed so that it can be
// applied to the database and to the in-memory model as two separate steps.
type policyMutation struct {
	op    policyOp
	sec   string
	ptype string

	// rules carries the rules for policyAdd and policyRemove.
	rules [][]string

	// fieldIndex and fieldValues carry the filter for policyRemoveFiltered.
	fieldIndex  int
	fieldValues []string
}

func addRules(sec string, ptype string, rules ...[]string) policyMutation {
	return policyMutation{op: policyAdd, sec: sec, ptype: ptype, rules: rules}
}

func removeRules(sec string, ptype string, rules ...[]string) policyMutation {
	return policyMutation{op: policyRemove, sec: sec, ptype: ptype, rules: rules}
}

func removeFiltered(sec string, ptype string, fieldIndex int, fieldValues ...string) policyMutation {
	return policyMutation{
		op:          policyRemoveFiltered,
		sec:         sec,
		ptype:       ptype,
		fieldIndex:  fieldIndex,
		fieldValues: fieldValues,
	}
}

// mutate applies mutations to the database now and to the in-memory policy
// model once the surrounding transaction commits.
//
// Splitting the two is what makes a policy change roll back with the write that
// caused it. The database half joins the caller's transaction through the
// adapter, so a rollback takes it with it. The in-memory half is registered to
// run after the commit, so a rollback leaves it having never happened — there is
// no compensating update to get wrong, and no window in which memory describes a
// transaction that did not survive.
//
// The database half runs without the enforcer lock. Holding it across database
// I/O would put a Go mutex and InnoDB row locks in one wait cycle, which
// InnoDB's deadlock detector cannot see and would surface only as a lock wait
// timeout. The memory half takes the lock once for the whole batch, so readers
// observe all of the mutations or none.
func (r *rbac) mutate(ctx context.Context, mutations ...policyMutation) error {
	if len(mutations) == 0 {
		return nil
	}
	if err := r.applyToStore(ctx, mutations); err != nil {
		return err
	}
	return database.AfterCommit(ctx, func(context.Context) error {
		return r.applyToModel(mutations)
	})
}

// applyToStore writes the mutations through the adapter, which resolves the
// context transaction per call.
func (r *rbac) applyToStore(ctx context.Context, mutations []policyMutation) error {
	for _, mutation := range mutations {
		var err error
		switch mutation.op {
		case policyAdd:
			err = r.adapter.AddPoliciesCtx(ctx, mutation.sec, mutation.ptype, mutation.rules)
		case policyRemove:
			err = r.adapter.RemovePoliciesCtx(ctx, mutation.sec, mutation.ptype, mutation.rules)
		case policyRemoveFiltered:
			err = r.adapter.RemoveFilteredPolicyCtx(
				ctx, mutation.sec, mutation.ptype, mutation.fieldIndex, mutation.fieldValues...,
			)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// applyToModel replays the mutations against the in-memory policy model under a
// single write lock.
//
// Role links are rebuilt from the rules the model reports as actually changed,
// not from the rules that were asked for: adding a rule that is already present
// changes nothing, and rebuilding links for it would be work the model does not
// agree happened.
//
// A failure here leaves memory behind the database, which the enforcer cannot
// repair on its own, so the model is reloaded from storage as a last resort.
func (r *rbac) applyToModel(mutations []policyMutation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	model := r.enforcer.GetModel()
	for _, mutation := range mutations {
		var affected [][]string
		var err error

		switch mutation.op {
		case policyAdd:
			affected, err = model.AddPoliciesWithAffected(mutation.sec, mutation.ptype, mutation.rules)
		case policyRemove:
			affected, err = model.RemovePoliciesWithAffected(mutation.sec, mutation.ptype, mutation.rules)
		case policyRemoveFiltered:
			_, affected, err = model.RemoveFilteredPolicy(
				mutation.sec, mutation.ptype, mutation.fieldIndex, mutation.fieldValues...,
			)
		}
		if err != nil {
			return errors.Join(err, r.reloadLocked())
		}

		if err = r.rebuildRoleLinks(mutation, affected); err != nil {
			return errors.Join(err, r.reloadLocked())
		}
	}
	return nil
}

// rebuildRoleLinks updates the role inheritance graph after a grouping change.
// Permission rules do not participate in it, and rebuilding for them would
// invalidate the matcher cache on every permission write for nothing.
func (r *rbac) rebuildRoleLinks(mutation policyMutation, affected [][]string) error {
	if mutation.sec != "g" || len(affected) == 0 {
		return nil
	}
	op := casbinmodel.PolicyAdd
	if mutation.op != policyAdd {
		op = casbinmodel.PolicyRemove
	}
	return r.enforcer.BuildIncrementalRoleLinks(op, mutation.ptype, affected)
}

// reloadLocked rebuilds the whole in-memory model from storage. It is the
// recovery path for a failed in-memory update, whose caller already holds the
// write lock.
func (r *rbac) reloadLocked() error {
	return errors.Wrap(r.enforcer.LoadPolicy(), "failed to reload casbin policies")
}
