package hdccsr

// InvalidationSet is the reverse 0/1/2-hop dependency cone of a vector mutation.
type InvalidationSet struct {
	Hop1Affected []uint16
	Hop2Affected []uint16
	AllAffected  map[uint16]bool
}

// ExpRevCSRGraph holds a reverse index for a FIXED graph topology.
// Rebuild this structure after inserting/removing any edge.
type ExpRevCSRGraph struct {
	TotalNodes int
	Offsets    []uint32
	Edges      []uint16
}

// BuildReverseCSR preprocesses inverse adjacency in O(V+E) time and space.
func BuildReverseCSR(graph *ExpCSRGraph) *ExpRevCSRGraph {
	n := graph.TotalNodes
	rev := &ExpRevCSRGraph{
		TotalNodes: n,
		Offsets:    make([]uint32, n+1),
		Edges:      make([]uint16, len(graph.Edges)),
	}
	for _, dst := range graph.Edges {
		rev.Offsets[int(dst)+1]++
	}
	for i := 1; i <= n; i++ {
		rev.Offsets[i] += rev.Offsets[i-1]
	}
	next := make([]uint32, n)
	copy(next, rev.Offsets[:n])
	for src := 0; src < n; src++ {
		for e := graph.Offsets[src]; e < graph.Offsets[src+1]; e++ {
			dst := graph.Edges[e]
			rev.Edges[next[dst]] = uint16(src)
			next[dst]++
		}
	}
	return rev
}

// FindAffectedCone2HopWithReverse enumerates only incoming 1-hop and 2-hop
// neighbors. This is O(deg_in(v) + sum(deg_in(u))), not a global graph scan.
func FindAffectedCone2HopWithReverse(rev *ExpRevCSRGraph, mutatedNode uint16) InvalidationSet {
	one := make([]uint16, 0)
	two := make([]uint16, 0)
	oneSeen := make(map[uint16]bool)
	twoSeen := make(map[uint16]bool)
	all := map[uint16]bool{mutatedNode: true}
	for a := rev.Offsets[mutatedNode]; a < rev.Offsets[int(mutatedNode)+1]; a++ {
		u := rev.Edges[a]
		if !oneSeen[u] {
			oneSeen[u] = true
			one = append(one, u)
			all[u] = true
		}
	}
	for _, u := range one {
		for a := rev.Offsets[u]; a < rev.Offsets[int(u)+1]; a++ {
			w := rev.Edges[a]
			if !twoSeen[w] {
				twoSeen[w] = true
				two = append(two, w)
				all[w] = true
			}
		}
	}
	return InvalidationSet{Hop1Affected: one, Hop2Affected: two, AllAffected: all}
}

// FindAffectedCone2Hop preserves the existing API, but builds a reverse index
// per invocation. For repeated edits prefer the WithReverse API below.
func FindAffectedCone2Hop(graph *ExpCSRGraph, mutatedNode uint16) InvalidationSet {
	return FindAffectedCone2HopWithReverse(BuildReverseCSR(graph), mutatedNode)
}

// RecomputeNodeHop1 exactly matches the single-node rules in PropagateHop1.
func (s *HDCSystem) RecomputeNodeHop1(graph *ExpCSRGraph, orig []Hypervector, v int) (Hypervector, bool) {
	if graph.Offsets[v] == graph.Offsets[v+1] {
		return Hypervector{}, false
	}
	messages := make([]Hypervector, 0, int(graph.Offsets[v+1]-graph.Offsets[v]))
	for e := graph.Offsets[v]; e < graph.Offsets[v+1]; e++ {
		rel := s.RelConfigs[graph.Relations[e]]
		messages = append(messages, Bind(RotL8K(orig[graph.Edges[e]], rel.Shift), rel.RoleVector))
	}
	return Majority(messages, s.TieBreaker)
}

// RecomputeNodeHop2 matches PropagateHop2 including its 1-hop fallback.
// Neither function uses hop1.Contextual for normal 2-hop messages: each
// composed message is derived from the ORIGINAL vector of the final node.
func (s *HDCSystem) RecomputeNodeHop2(graph *ExpCSRGraph, orig []Hypervector, v int) (Hypervector, bool) {
	startV, endV := graph.Offsets[v], graph.Offsets[v+1]
	if startV == endV {
		return Hypervector{}, false
	}
	messages := make([]Hypervector, 0, 16)
	for eV := startV; eV < endV; eV++ {
		u := graph.Edges[eV]
		cfg1 := s.RelConfigs[graph.Relations[eV]]
		for eU := graph.Offsets[u]; eU < graph.Offsets[int(u)+1]; eU++ {
			w := graph.Edges[eU]
			if int(w) == v {
				continue
			}
			cfg2 := s.RelConfigs[graph.Relations[eU]]
			role := Bind(RotL8K(cfg2.RoleVector, cfg1.Shift), cfg1.RoleVector)
			shift := cfg1.Shift + cfg2.Shift
			messages = append(messages, Bind(RotL8K(orig[w], shift), role))
		}
	}
	if len(messages) == 0 {
		// Critical: canonical PropagateHop2 falls back to hop1 for any
		// non-leaf source with no non-backtracking paths of length 2.
		return s.RecomputeNodeHop1(graph, orig, v)
	}
	return Majority(messages, s.TieBreaker)
}

// UpdateNodeIncremental2HopWithReverse is the fast vector-only update.
// The caller owns the reverse index and must rebuild it after any topology
// change. In-place mutation requires exclusive access from other readers.
func (s *HDCSystem) UpdateNodeIncremental2HopWithReverse(
	graph *ExpCSRGraph,
	reverse *ExpRevCSRGraph,
	currentMem *ContextualMemory,
	mutatedNode uint16,
	newVec Hypervector,
) (*ContextualMemory, InvalidationSet) {
	affected := FindAffectedCone2HopWithReverse(reverse, mutatedNode)
	currentMem.Original[mutatedNode] = newVec
	for node := range affected.AllAffected {
		ctx, has := s.RecomputeNodeHop2(graph, currentMem.Original, int(node))
		currentMem.Contextual[node] = ctx
		currentMem.HasContext[node] = has
	}
	return currentMem, affected
}

// UpdateNodeIncremental2Hop is the compatibility API. For repeated edits
// use UpdateNodeIncremental2HopWithReverse to amortize BuildReverseCSR.
func (s *HDCSystem) UpdateNodeIncremental2Hop(
	graph *ExpCSRGraph,
	currentMem *ContextualMemory,
	mutatedNode uint16,
	newVec Hypervector,
) (*ContextualMemory, InvalidationSet) {
	return s.UpdateNodeIncremental2HopWithReverse(
		graph, BuildReverseCSR(graph), currentMem, mutatedNode, newVec,
	)
}
