package rbac

// roleGraph answers role reachability: who holds which role, directly or
// through another role, per tenant for the g grouping and globally for g2.
//
// It replaces Casbin's role managers, and keeps their contract where the
// package consumed it: hasLink resolves transitive membership up to
// maxRoleHierarchy links, a self-match reports true and is guarded by the
// caller, and directRoles answers one hop only — which is what denyReason
// distinguishes its two answers by.
//
// Like the decision index it is immutable once built and rebuilt whole from
// the grouping rules whenever they change, under the same lock, for the same
// reason: grouping writes happen at administrative frequency, and a
// derivation with no update paths has no update bugs.
type roleGraph struct {
	// tenant maps domain → member → the roles held directly in that domain.
	tenant map[string]map[string][]string

	// system maps member → the system-level roles held directly.
	system map[string][]string
}

// policyGraph is the graph decisions and membership reads currently answer
// from. It is written under the policy write lock wherever the rule set
// changes, and read under the read lock those paths already hold.
var policyGraph *roleGraph

// buildRoleGraph derives a graph from the grouping rules set holds.
func buildRoleGraph(set *policySet) *roleGraph {
	graph := &roleGraph{
		tenant: make(map[string]map[string][]string),
		system: make(map[string][]string),
	}
	for _, rule := range set.all(tenantRoleGrouping) {
		domain := graph.tenant[rule[2]]
		if domain == nil {
			domain = make(map[string][]string)
			graph.tenant[rule[2]] = domain
		}
		domain[rule[0]] = append(domain[rule[0]], rule[1])
	}
	for _, rule := range set.all(systemRoleGrouping) {
		graph.system[rule[0]] = append(graph.system[rule[0]], rule[1])
	}
	return graph
}

// directRoles reports the roles name holds directly through grouping, inside
// domain for the tenant grouping and ignoring domain for the system one.
func (g *roleGraph) directRoles(grouping string, name string, domain string) []string {
	if grouping == systemRoleGrouping {
		return g.system[name]
	}
	return g.tenant[domain][name]
}

// hasLink reports whether subject reaches role through grouping, walking at
// most maxRoleHierarchy links — the same cap Casbin's role managers were
// built with. A subject reaching for its own name reports true, mirroring
// the managers' self-match; every caller guards that case before asking.
func (g *roleGraph) hasLink(grouping string, subject string, role string, domain string) bool {
	if subject == role {
		return true
	}
	seen := map[string]struct{}{subject: {}}
	frontier := []string{subject}
	for level := 0; level < maxRoleHierarchy && len(frontier) > 0; level++ {
		next := make([]string, 0, len(frontier))
		for _, name := range frontier {
			for _, held := range g.directRoles(grouping, name, domain) {
				if held == role {
					return true
				}
				if _, ok := seen[held]; ok {
					continue
				}
				seen[held] = struct{}{}
				next = append(next, held)
			}
		}
		frontier = next
	}
	return false
}
