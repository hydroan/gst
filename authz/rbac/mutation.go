package rbac

import (
	"context"
	"sync/atomic"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	"github.com/hydroan/gst/logger"
	"go.uber.org/zap"
)

// policyStorage is the storage mutate drives and reload reads back: batched
// writes, and removal entry points that report how many stored rows went.
//
// The counts exist to be compared. A removal is the one operation whose effect
// in storage and in memory can silently differ — a backend collation treating
// two rules as one, or another transaction's uncommitted insert a delete
// passes by — and the row count is the one thing both halves can state about
// what they did. Insertions cannot be compared this way: MySQL reports an
// insert that hit the conflict clause as a found row, so an add has no count
// that means the same thing on every backend.
type policyStorage interface {
	loadPolicies(ctx context.Context) (*policySet, error)
	addPolicies(ctx context.Context, ptype string, rules [][]string) error
	removePoliciesCount(ctx context.Context, ptype string, rules [][]string) (int64, error)
	removeFilteredPolicyCount(ctx context.Context, ptype string, fieldIndex int, fieldValues ...string) (int64, error)
}

// storedCountUnknown is the count of a mutation whose stored effect cannot be
// compared: an insertion, whose count means different things per backend. A
// mutation carrying it is applied without the check.
const storedCountUnknown int64 = -1

// policyOp is what a mutation does to the stored policies.
type policyOp int

const (
	policyAdd policyOp = iota
	policyRemove
	policyRemoveFiltered
)

// policyMutation is one change to the policy store, expressed so that it can be
// applied to the database and to the in-memory set as two separate steps.
type policyMutation struct {
	op    policyOp
	ptype string

	// rules carries the rules for policyAdd and policyRemove.
	rules [][]string

	// fieldIndex and fieldValues carry the filter for policyRemoveFiltered.
	fieldIndex  int
	fieldValues []string
}

func addRules(ptype string, rules ...[]string) policyMutation {
	return policyMutation{op: policyAdd, ptype: ptype, rules: rules}
}

func removeRules(ptype string, rules ...[]string) policyMutation {
	return policyMutation{op: policyRemove, ptype: ptype, rules: rules}
}

func removeFiltered(ptype string, fieldIndex int, fieldValues ...string) policyMutation {
	return policyMutation{
		op:          policyRemoveFiltered,
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
// The database half runs without the policy lock. Holding it across database
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
		storedCounts, err := r.applyToStore(ctx, mutations)
		if err != nil {
			return err
		}
		// Numbered here, after the rows are written and before the commit, so
		// that the memory half can tell whether it is still the newest word on
		// what it changed. See policySequence.
		sequence := policySequence.Add(1)
		return database.AfterCommit(ctx, func(ctx context.Context) error {
			return r.applyToModel(ctx, sequence, mutations, storedCounts)
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
// taken. It is read and written only under the policy write lock, which every
// applier holds for the whole of its update.
var appliedSequence uint64

// applyToStore writes the mutations through the adapter, which resolves the
// context transaction per call. It reports how many stored rows each removal
// affected, and storedCountUnknown for the mutations that cannot be compared.
func (r *rbac) applyToStore(ctx context.Context, mutations []policyMutation) ([]int64, error) {
	storedCounts := make([]int64, len(mutations))
	for i, mutation := range mutations {
		var err error
		storedCounts[i] = storedCountUnknown
		switch mutation.op {
		case policyAdd:
			err = r.adapter.addPolicies(ctx, mutation.ptype, mutation.rules)
		case policyRemove:
			storedCounts[i], err = r.adapter.removePoliciesCount(ctx, mutation.ptype, mutation.rules)
		case policyRemoveFiltered:
			storedCounts[i], err = r.adapter.removeFilteredPolicyCount(
				ctx, mutation.ptype, mutation.fieldIndex, mutation.fieldValues...,
			)
		}
		if err != nil {
			return nil, err
		}
	}
	return storedCounts, nil
}

// applyToModel brings the in-memory policy model up to date with a batch that
// storage has already accepted, rebuilding it from storage when it cannot.
func (r *rbac) applyToModel(
	ctx context.Context, sequence uint64, mutations []policyMutation, storedCounts []int64,
) error {
	applied, cause := r.applyMutations(sequence, mutations, storedCounts)
	if applied {
		return nil
	}
	return r.recoverPolicies(ctx, cause)
}

// applyMutations replays the mutations against the in-memory policy set under
// a single write lock, rebuilds what is derived from it, and reports whether
// memory is still in step with storage afterwards. A false report carries the
// failure that caused it, or nil when the set has to be rebuilt from storage
// without anything having failed: a batch that was overtaken, or a removal
// whose two halves disagreed.
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
//
// A removal that affected storage and memory differently stops the replay the
// same way. The two halves can part company without either erring: a backend
// collation reading two rules as one deletes a row the caller did not name, and
// on a backend whose statements see only committed rows, a delete passes by the
// rule an uncommitted insert is adding. Which rules storage actually holds
// cannot be derived from the counts, so the model is rebuilt from storage —
// which also puts right any older divergence the counts happened to surface.
func (r *rbac) applyMutations(sequence uint64, mutations []policyMutation, storedCounts []int64) (bool, error) {
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

	set := policyRules
	for i, mutation := range mutations {
		var affected [][]string
		switch mutation.op {
		case policyAdd:
			affected = set.add(mutation.ptype, mutation.rules)
		case policyRemove:
			affected = set.remove(mutation.ptype, mutation.rules)
		case policyRemoveFiltered:
			affected = set.removeFiltered(mutation.ptype, mutation.fieldIndex, mutation.fieldValues...)
		}

		if storedCounts[i] != storedCountUnknown && storedCounts[i] != int64(len(affected)) {
			logger.Authz.Warnz(
				"rbac policy removal affected storage and memory differently, reloading from storage",
				zap.String("ptype", mutation.ptype),
				zap.Int64("stored_rows", storedCounts[i]),
				zap.Int("memory_rules", len(affected)),
			)
			return false, nil
		}
	}
	rebuildDerived(set)
	return true, nil
}
