// Package rbac decides authorization from Casbin policies and keeps those
// policies in step with the records they are derived from.
//
// Three rule kinds carry everything the package stores. A permission,
// (tenant, role, object, action, effect), is what a role may reach. An
// assignment, (subject, role, tenant), is who holds a role in one tenant. A
// system assignment, (subject, role), is who holds a role above every tenant,
// which is how the built-in root subject stays reachable in a deployment with no
// tenants configured at all.
//
// Reads answer from memory: the two role branches from the role graph, and
// everything a policy decides from the enforcer. Writes do not: they go through
// mutate, which drives storage and memory as two halves so that a policy change
// rolls back with the transaction that caused it. Nothing in the package writes
// through the enforcer's own AddPolicy family, and autosave stays off so Casbin
// cannot write behind mutate's back.
//
// # One process decides from its own memory
//
// A decision is answered from the policy set this process holds, and only the
// process that performed a write updates that set. Nothing tells one process
// about a write another one made.
//
// With a single process serving requests, that is the whole story: every write
// goes through mutate, so the one policy set in existence is always current.
// Two things still reach past it, and both are visible rather than silent —
// ReloadPolicies puts either right, and restarting does too:
//
//   - Storage changed by something other than this process: a manual repair, a
//     restore, another tool writing the policy table. A removal that touches
//     the affected rules surfaces it early — its stored and in-memory row
//     counts disagree, and the disagreement is answered by reloading.
//   - A recovery that could not read storage back. The process then serves
//     decisions that disagree with what is stored, publishes that state through
//     the authz policy divergence gauge, logs it at error level, and keeps
//     retrying the reload until one succeeds.
//
// With more than one process sharing a policy table — replicas behind a load
// balancer, a service scaled beyond one instance — it becomes the dominant
// failure. A revocation applied on one replica never reaches the others, they
// keep allowing what was revoked, and nothing reports it: the divergence gauge
// covers a failed reload in this process, not being behind another one. There
// is no error, no metric, and no log to notice.
//
// TODO: converge the in-memory policy set across processes before running more
// than one. The mechanism has to be level-triggered to be worth having —
// periodic reconciliation against storage, so that a missed signal costs
// latency rather than correctness. A published notification (Redis pub/sub or
// equivalent) can sit on top to cut the delay, but cannot be the only
// mechanism: it is at-most-once, and it would rest on a component the framework
// treats as optional. Both feed the same ReloadPolicies entry point, and both
// should treat the divergence state as a second reason to reload.
//
// # Storage decides what two rules are, and does not always agree
//
// A rule's identity in memory is its exact bytes, because Casbin keys the
// in-memory set with a Go map. Its identity in storage is whatever the column's
// collation says, and the framework supports backends that disagree: a
// case-insensitive collation, which is the default in some MySQL installations,
// treats two rules differing only in case as one, while memory treats them as
// two.
//
// Where they disagree, the two halves of a write part company. An exact delete
// removes a rule the caller did not name and leaves the named one in memory —
// that half is caught, because the two sides then count different rules and
// the disagreement is answered by reloading. An insert whose rule collides
// with a stored one under that collation is dropped by the on-conflict clause
// and kept in memory, and that half stays silent: an insert has no row count
// that means the same thing on every backend, so there is nothing to compare.
//
// TODO: stop depending on storage to decide rule identity, in a way that holds
// for every supported backend. Nothing here can require a collation, since the
// database is not the framework's to configure.
package rbac
