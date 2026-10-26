package valuevalidation

import (
	"fmt"
	"slices"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

// adjacency is a conventional indexed BFS reference. It shares only the typed
// symbol/edge data, not the CSR offsets, traversal or production report method.
type adjacency struct {
	graph   *impact.ModuleGraph
	ids     map[string]uint32
	parents map[uint32][]uint32
}

func newAdjacency(g *impact.ModuleGraph) (*adjacency, error) {
	a := &adjacency{graph: g, ids: map[string]uint32{}, parents: map[uint32][]uint32{}}
	for i, name := range g.Symbols {
		if _, exists := a.ids[name]; exists {
			return nil, fmt.Errorf("duplicate typed identity %s", name)
		}
		a.ids[name] = uint32(i)
	}
	for _, edge := range g.Edges {
		caller, ok := a.ids[edge.Caller]
		if !ok {
			return nil, fmt.Errorf("unknown caller %s", edge.Caller)
		}
		callee, ok := a.ids[edge.Callee]
		if !ok {
			return nil, fmt.Errorf("unknown callee %s", edge.Callee)
		}
		a.parents[callee] = append(a.parents[callee], caller)
	}
	return a, nil
}

func (a *adjacency) query(symbol string) (impact.ModuleImpact, error) {
	root, ok := a.ids[symbol]
	if !ok {
		return impact.ModuleImpact{}, fmt.Errorf("unknown symbol %s", symbol)
	}
	g := a.graph
	out := impact.ModuleImpact{Schema: "mbvm-module-impact-v1", Symbol: symbol, Packages: len(g.Packages), Symbols: len(g.Symbols), ResolvedEdges: len(g.Edges), UnlinkedImports: g.UnlinkedImports, DirectCallers: []string{}, TransitiveCandidates: []string{}, Warnings: []string{
		"Only loaded local Go packages and statically resolved call sites are included.",
		"Interface dispatch, function values and reflection are not assigned to concrete callees.",
		"Graph reachability is not proof of compile-time or runtime breakage.",
	}}
	seen := make([]bool, len(g.Symbols))
	queue := make([]uint32, len(g.Symbols))
	depth := make([]int, len(g.Symbols))
	seen[root] = true
	queue[0] = root
	for head, tail := 0, 1; head < tail; head++ {
		id := queue[head]
		out.MaxDepth = max(out.MaxDepth, depth[head])
		for _, parent := range a.parents[id] {
			if seen[parent] {
				continue
			}
			seen[parent] = true
			queue[tail] = parent
			depth[tail] = depth[head] + 1
			tail++
			if depth[head] == 0 {
				out.DirectCallers = append(out.DirectCallers, g.Symbols[parent])
			} else {
				out.TransitiveCandidates = append(out.TransitiveCandidates, g.Symbols[parent])
			}
		}
	}
	slices.Sort(out.DirectCallers)
	slices.Sort(out.TransitiveCandidates)
	return out, nil
}

func equalImpact(a, b impact.ModuleImpact) bool {
	return a.Schema == b.Schema && a.Symbol == b.Symbol && a.Packages == b.Packages && a.Symbols == b.Symbols && a.ResolvedEdges == b.ResolvedEdges && a.UnlinkedImports == b.UnlinkedImports && a.MaxDepth == b.MaxDepth && slices.Equal(a.DirectCallers, b.DirectCallers) && slices.Equal(a.TransitiveCandidates, b.TransitiveCandidates) && slices.Equal(a.Warnings, b.Warnings)
}
