package rbac

import (
	"context"
	"sync/atomic"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
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

// ErrEmptyPolicyValue reports a mutation carrying an empty value where an
// identifier belongs.
//
// An empty value does not narrow a request, it widens it. Storage and Casbin
// agree that an empty filter field matches anything, so a filtered delete built
// from an empty argument removes every rule of its kind in the deployment
// instead of none — revoking a role for one subject would revoke it for all of
// them. An empty value inside a rule is the mirror image: the row stores fewer
// meaningful columns than its kind declares, and the next load of the policy set
// rejects it and takes the whole set down with it.
//
// Neither outcome can be recovered by guessing what was meant, and both are
// silent at the call site, so a mutation carrying an empty value is refused
// before anything is written.
var ErrEmptyPolicyValue = errors.New("rbac: policy mutation carries an empty value")

// validate reports whether the mutation can be applied without widening or
// corrupting the policy set. See ErrEmptyPolicyValue.
//
// Every rule kind the framework writes is fully populated: a permission is
// (tenant, role, object, action, effect), an assignment is (subject, role,
// tenant), and a system assignment is (subject, role). None of them has a column
// that is meaningfully empty, so an empty value always means the caller passed
// one in.
func (m policyMutation) validate() error {
	switch m.op {
	case policyAdd, policyRemove:
		for _, rule := range m.rules {
			for i, value := range rule {
				if value == "" {
					return errors.Wrapf(ErrEmptyPolicyValue, "%s rule field %d", m.ptype, i)
				}
			}
		}
	case policyRemoveFiltered:
		if len(m.fieldValues) == 0 {
			return errors.Wrapf(ErrEmptyPolicyValue, "%s filter has no values", m.ptype)
		}
		for i, value := range m.fieldValues {
			if value == "" {
				return errors.Wrapf(ErrEmptyPolicyValue, "%s filter field %d", m.ptype, m.fieldIndex+i)
			}
		}
	}
	return nil
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
//
// It opens a transaction of its own when the caller has none, rather than
// leaving the caller to remember one. A mutation set is one logical change —
// replacing a role's permissions is a delete and an insert — and separately
// autocommitting its parts lets a failed insert leave the role stripped of every
// permission while the memory half, which never ran, keeps serving the old set.
// That divergence outlives the process that caused it and is invisible to a
// comparison of stored state, so the correct call has to be the only call.
// Joining an existing transaction costs nothing, which is what makes requiring
// one here affordable.
func (r *rbac) mutate(ctx context.Context, mutations ...policyMutation) error {
	if len(mutations) == 0 {
		return nil
	}
	for _, mutation := range mutations {
		if err := mutation.validate(); err != nil {
			return err
		}
	}
	return database.Transaction(ctx, func(ctx context.Context) error {
		if err := r.applyToStore(ctx, mutations); err != nil {
			return err
		}
		// Numbered here, after the rows are written and before the commit, so
		// that the memory half can tell whether it is still the newest word on
		// what it changed. See policySequence.
		sequence := policySequence.Add(1)
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			return r.applyToModel(ctx, sequence, mutations)
		})
	})
}

// policySequence orders policy writes by the order storage accepted them.
//
// A batch takes its number after its rows are written and before its
// transaction commits. Row locks make that placement meaningful: a batch
// overwriting rules another batch is holding cannot reach its own numbering
// until that other batch commits and releases them. So for any two batches
// whose rules overlap — the only pairs whose order can matter — the numbers run
// the same way storage settled it.
//
// The counter is per process, which is the same reach the in-memory model has.
// Nothing here claims to order writes against another replica's memory; a
// replica learns of writes it did not make only by reloading.
var policySequence atomic.Uint64

// appliedSequence is the number of the newest batch the in-memory model has
// taken. It is read and written only under the enforcer write lock, which every
// applier holds for the whole of its update.
var appliedSequence uint64

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
//
// A batch overtaken between its commit and this point does not replay at all.
// Nothing orders the memory halves of two transactions against each other: the
// commit settles their order in storage, and which goroutine then reaches the
// lock first is the scheduler's business. Replaying an overtaken batch would
// undo the newer one in memory only, leaving decisions that disagree with
// storage for as long as the process runs. Its rules cannot simply be dropped
// either — the newer batch may have changed different rules entirely — so the
// model is rebuilt from storage, which is the one answer that is right whatever
// the two batches touched.
func (r *rbac) applyToModel(ctx context.Context, sequence uint64, mutations []policyMutation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	overtaken := sequence < appliedSequence
	appliedSequence = max(appliedSequence, sequence)
	if overtaken {
		logger.Authz.Warnz(
			"rbac policy write was overtaken before reaching memory, reloading from storage",
			zap.Uint64("sequence", sequence),
			zap.Uint64("applied_sequence", appliedSequence),
		)
		return r.reloadLocked(ctx)
	}

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
			return r.recoverLocked(ctx, err)
		}

		if err = r.rebuildRoleLinks(mutation, affected); err != nil {
			return r.recoverLocked(ctx, err)
		}
	}
	return nil
}

// recoverLocked rebuilds the model from storage after an in-memory update
// failed, and records both what failed and whether the rebuild rescued it.
//
// This is the one place where the decisions a process serves can disagree with
// what is stored: the transaction is already durable, so the write cannot be
// undone, and only memory is behind. Nothing downstream notices — the request
// that caused it has usually returned, and comparing stored rules against the
// records they come from cannot see a divergence that exists only in memory. It
// is logged here because here is the only place that knows.
func (r *rbac) recoverLocked(ctx context.Context, cause error) error {
	reloaded := r.reloadLocked(ctx)
	if reloaded == nil {
		logger.Authz.Warnz(
			"rbac in-memory policy update failed, reloaded from storage",
			zap.Error(cause),
		)
		return cause
	}
	logger.Authz.Errorz(
		"rbac in-memory policy update failed and could not be reloaded, "+
			"authorization decisions now disagree with stored policies until this process reloads them",
		zap.Error(cause),
		zap.NamedError("reload_error", reloaded),
	)
	return errors.Join(cause, reloaded)
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

// ReloadPolicies rebuilds the whole in-memory model from storage.
func (r *rbac) ReloadPolicies(ctx context.Context) (err error) {
	ctx, finishSpan := traceRBAC(ctx, "reload_policies", nil)
	defer func() {
		finishSpan(err)
	}()

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reloadLocked(ctx)
}

// reloadLocked rebuilds the whole in-memory model from storage. It is the
// recovery path for a failed in-memory update, whose caller already holds the
// write lock.
func (r *rbac) reloadLocked(ctx context.Context) error {
	return errors.Wrap(r.enforcer.LoadPolicyCtx(ctx), "failed to reload casbin policies")
}
