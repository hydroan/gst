package rbac

import "strings"

// tenantRoleGrouping holds assignments inside one tenant, systemRoleGrouping
// those that sit above every tenant. They are the ptype values assignment
// rules are stored under, and two of the kinds ruleTokens declares.
const (
	tenantRoleGrouping = "g"
	systemRoleGrouping = "g2"
)

// policyTable is the table the policy adapter reads and writes. Its schema is
// owned by the AuthzRule model, so the name has to agree with that model's
// TableName.
const policyTable = "authz_rules"

// ruleTokens fixes how many values each rule kind carries: a permission is
// (tenant, role, obj, act, eft), a tenant assignment (subject, role, tenant),
// a system assignment (subject, role). It is what the Casbin model's
// assertions used to declare, kept as the one table every consumer sizes and
// validates rules against — the loader when it shapes a stored row, and the
// set below when it refuses a kind it does not know.
var ruleTokens = map[string]int{
	"p":                5,
	tenantRoleGrouping: 3,
	systemRoleGrouping: 2,
}

// rulePtypes lists the rule kinds in a fixed order, for consumers that walk
// the whole set and need the walk to be the same on every run.
var rulePtypes = []string{"p", tenantRoleGrouping, systemRoleGrouping}

// policySet is the in-memory policy store: every rule the process decides
// from, held per kind, in insertion order, keyed by its exact values.
//
// It replaces the Casbin model as the container the two halves of a write
// meet in. The properties the package depends on are the ones Casbin's
// container had: a rule's identity is its exact bytes, adding an existing
// rule changes nothing, removals report what actually went — which is what
// the stored-count comparison reads — and the order rules were added in is
// preserved, because it decides which rule a decision names.
//
// It is not safe for concurrent use on its own; every reader and writer holds
// the policy lock, which is the package-wide discipline.
type policySet struct {
	rules map[string][][]string
	seen  map[string]map[string]struct{}
}

func newPolicySet() *policySet {
	return &policySet{
		rules: make(map[string][][]string, len(ruleTokens)),
		seen:  make(map[string]map[string]struct{}, len(ruleTokens)),
	}
}

// ruleKey joins a rule into its identity. The separator cannot appear in a
// value that came through mutate, whose validation refuses empty values but
// not control bytes; a value carrying it could only alias another rule in
// memory, while storage keeps them apart, and the stored-count comparison
// surfaces exactly that kind of disagreement.
func ruleKey(rule []string) string { return strings.Join(rule, "\x00") }

// add stores the rules that are not already present and reports them.
func (s *policySet) add(ptype string, rules [][]string) [][]string {
	seen := s.seen[ptype]
	if seen == nil {
		seen = make(map[string]struct{})
		s.seen[ptype] = seen
	}
	affected := make([][]string, 0, len(rules))
	for _, rule := range rules {
		key := ruleKey(rule)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		s.rules[ptype] = append(s.rules[ptype], rule)
		affected = append(affected, rule)
	}
	return affected
}

// remove deletes the rules by their exact values and reports the ones that
// were present to delete.
func (s *policySet) remove(ptype string, rules [][]string) [][]string {
	seen := s.seen[ptype]
	if len(seen) == 0 {
		return nil
	}
	removing := make(map[string]struct{}, len(rules))
	affected := make([][]string, 0, len(rules))
	for _, rule := range rules {
		key := ruleKey(rule)
		if _, ok := seen[key]; !ok {
			continue
		}
		delete(seen, key)
		removing[key] = struct{}{}
	}
	if len(removing) == 0 {
		return nil
	}
	kept := s.rules[ptype][:0]
	for _, rule := range s.rules[ptype] {
		if _, ok := removing[ruleKey(rule)]; ok {
			affected = append(affected, rule)
			continue
		}
		kept = append(kept, rule)
	}
	s.rules[ptype] = kept
	return affected
}

// removeFiltered deletes the rules matching fieldValues starting at
// fieldIndex and reports them. An empty value matches any value in that
// position, which is the filter semantics the storage side applies, and the
// two have to agree or their counts read as a disagreement to repair.
func (s *policySet) removeFiltered(ptype string, fieldIndex int, fieldValues ...string) [][]string {
	rules := s.rules[ptype]
	if len(rules) == 0 {
		return nil
	}
	affected := make([][]string, 0)
	kept := rules[:0]
	for _, rule := range rules {
		if ruleMatchesFilter(rule, fieldIndex, fieldValues) {
			affected = append(affected, rule)
			delete(s.seen[ptype], ruleKey(rule))
			continue
		}
		kept = append(kept, rule)
	}
	s.rules[ptype] = kept
	return affected
}

func ruleMatchesFilter(rule []string, fieldIndex int, fieldValues []string) bool {
	for i, value := range fieldValues {
		if value == "" {
			continue
		}
		position := fieldIndex + i
		if position >= len(rule) || rule[position] != value {
			return false
		}
	}
	return true
}

// filtered reports the rules whose value at fieldIndex equals value, in
// insertion order.
func (s *policySet) filtered(ptype string, fieldIndex int, value string) [][]string {
	matched := make([][]string, 0)
	for _, rule := range s.rules[ptype] {
		if fieldIndex < len(rule) && rule[fieldIndex] == value {
			matched = append(matched, rule)
		}
	}
	return matched
}

// all reports the rules of one kind, in insertion order. The slice is the
// set's own storage; callers iterate and do not edit.
func (s *policySet) all(ptype string) [][]string { return s.rules[ptype] }
