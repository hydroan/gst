package modelregistry

import (
	"reflect"
	"sync"
)

// replicaPreferrer is the optional model capability behind PrefersReplica: a
// model declares that its reads default to a read replica by implementing
//
//	func (*Report) PreferReplica() bool { return true }
//
// The declaration means "every read of this model tolerates replication
// staleness" — audit trails, historical ledgers, report rows. It is a
// default, not a mandate: a call site takes a single read back to the
// primary with WithReplica(false), a transaction always stays on the
// primary, and a deployment without configured replicas serves the reads
// from the primary anyway. Writes are never affected. See the WithReplica
// documentation for the full precedence.
type replicaPreferrer interface{ PreferReplica() bool }

// replicaPreferenceCache memoizes the per-type answer; the method set of a
// type is fixed for the life of the binary.
var replicaPreferenceCache sync.Map

// PrefersReplica reports whether m's model type declared PreferReplica()
// true. m may be a nil pointer — only its type is consulted, and the answer
// is resolved once per type on a fresh instance, so the method body may
// safely read receiver fields.
func PrefersReplica(m any) bool {
	typ := reflect.TypeOf(m)
	if typ == nil {
		return false
	}
	if cached, ok := replicaPreferenceCache.Load(typ); ok {
		return cached.(bool) //nolint:errcheck
	}

	elem := typ
	for elem.Kind() == reflect.Pointer {
		elem = elem.Elem()
	}
	prefers := false
	if elem.Kind() == reflect.Struct {
		if preferrer, ok := reflect.TypeAssert[replicaPreferrer](reflect.New(elem)); ok {
			prefers = preferrer.PreferReplica()
		}
	}
	replicaPreferenceCache.Store(typ, prefers)
	return prefers
}
