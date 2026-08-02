package rbac

import (
	"context"
	"sync/atomic"
	"time"

	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	prommetrics "github.com/hydroan/gst/metrics"
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

// applyToModel brings the in-memory policy model up to date with a batch that
// storage has already accepted, rebuilding it from storage when it cannot.
func (r *rbac) applyToModel(ctx context.Context, sequence uint64, mutations []policyMutation) error {
	applied, cause := r.applyMutations(sequence, mutations)
	if applied {
		return nil
	}
	return r.recover(ctx, cause)
}

// applyMutations replays the mutations against the in-memory policy model under
// a single write lock, and reports whether the model is still in step with
// storage afterwards. A false report carries the failure that caused it, or nil
// when the batch was overtaken, which is not a failure.
//
// Role links are rebuilt from the rules the model reports as actually changed,
// not from the rules that were asked for: adding a rule that is already present
// changes nothing, and rebuilding links for it would be work the model does not
// agree happened.
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
func (r *rbac) applyMutations(sequence uint64, mutations []policyMutation) (bool, error) {
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
		return false, nil
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
			return false, err
		}

		if err = r.rebuildRoleLinks(mutation, affected); err != nil {
			return false, err
		}
	}
	return true, nil
}

// reloadTimeout bounds a reload that has been cut loose from what asked for it.
//
// It is pulled between two limits. It has to outlast the client whose request
// triggered the reload, which is the whole reason the context is detached. It
// also has to stay well under what anything upstream will wait for, because an
// after-commit action runs on the request's own goroutine and holds the
// enforcer write lock while it reads: every authorization in the process waits
// behind it. A read of the policy table is sub-second on a database that is
// answering, so this only ever bites on one that is not, and holding a request
// and the whole authorization path hostage for longer helps nobody.
const reloadTimeout = 10 * time.Second

// recover rebuilds the model from storage after an in-memory update was
// overtaken or failed, and records whether the rebuild rescued this process.
//
// This is the one place where the decisions a process serves can disagree with
// what is stored: the transaction is already durable, so the write cannot be
// undone, and only memory is behind. Nothing downstream notices — the request
// that caused it has usually returned, and comparing stored rules against the
// records they come from cannot see a divergence that exists only in memory. It
// is recorded here because here is the only place that knows.
//
// The reload is given a context of its own. This runs from an after-commit
// action, which receives the context from before the transaction opened — in an
// HTTP handler, the request's. A client that has already disconnected would
// otherwise cancel the one read that can put this process back in step with
// storage, which is how a reload comes to fail most often.
func (r *rbac) recover(ctx context.Context, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(contextOrBackground(ctx)), reloadTimeout)
	defer cancel()

	reloaded := r.reload(ctx)
	if reloaded == nil {
		if cause != nil {
			logger.Authz.Warnz(
				"rbac in-memory policy update failed, reloaded from storage",
				zap.Error(cause),
			)
		}
		return cause
	}

	publishPolicyDivergence(true)
	// The two callers reach here for different reasons, and saying "the update
	// failed" for a batch that was deliberately not replayed would send whoever
	// reads this looking for a failure that never happened. An overtaken batch
	// also carries no cause, and zap drops a nil error field, so that reading
	// would arrive with nothing at all to explain itself.
	if cause == nil {
		logger.Authz.Errorz(
			"rbac policy write was overtaken and the reload it needed failed, "+
				"authorization decisions now disagree with stored policies until this process reloads them",
			zap.NamedError("reload_error", reloaded),
		)
		return reloaded
	}
	logger.Authz.Errorz(
		"rbac in-memory policy update failed and could not be reloaded, "+
			"authorization decisions now disagree with stored policies until this process reloads them",
		zap.Error(cause),
		zap.NamedError("reload_error", reloaded),
	)
	return errors.Join(cause, reloaded)
}

// policiesDiverged reports whether this process serves authorization decisions
// from an in-memory policy set that no longer agrees with storage.
var policiesDiverged atomic.Bool

// publishPolicyDivergence records whether this process is still in step with
// storage, so that something outside it can act on the answer.
func publishPolicyDivergence(diverged bool) {
	policiesDiverged.Store(diverged)

	// The gauge is created by prommetrics.Init, which bootstrap runs long
	// before anything can write a policy. A process that never ran bootstrap,
	// which is what a test is, still has to be able to reload.
	if prommetrics.AuthzPolicyDiverged == nil {
		return
	}
	if diverged {
		prommetrics.AuthzPolicyDiverged.Set(1)
		return
	}
	prommetrics.AuthzPolicyDiverged.Set(0)
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

	return r.reload(ctx)
}

// reload rebuilds the whole in-memory policy set from storage.
//
// The read is held under the write lock, which stalls every authorization in
// the process for as long as it takes. That is deliberate, and it was arrived
// at the other way round: reading outside the lock and installing the result
// afterwards was tried and reverted. The applied sequence counts only the
// batches this process wrote, so it cannot order one reload against another,
// and two overlapping reloads each pass a check against it — leaving the slower
// one to install its older snapshot over the newer. Storage changed by anything
// other than this process, which is what ReloadPolicies exists for, is exactly
// where nothing local would have moved that sequence to catch it.
//
// Holding the lock also keeps the stale set from being served while its
// replacement is read. A reload usually follows a revocation that memory
// missed, so the decisions that window would serve are the ones already known
// to be wrong, and making requests wait is the safer way to be unavailable.
//
// The enforcer's own load is what makes this usable at all: it swaps the model
// through applyModifiedModel rather than SetModel. SetModel re-initializes the
// enforcer, rebuilding the function map so that no decision can resolve the
// matcher function this package registers, and turning autosave back on so that
// Casbin writes policies behind mutate's back.
func (r *rbac) reload(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.enforcer.LoadPolicyCtx(ctx); err != nil {
		return errors.Wrap(err, "failed to reload casbin policies")
	}
	publishPolicyDivergence(false)
	return nil
}
