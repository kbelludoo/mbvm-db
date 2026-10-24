package erdosstraus

import (
	"fmt"
	"slices"
)

// RelationType categorizes edges in the mathematical CSR graph.
type RelationType uint8

const (
	RelDependency   RelationType = 1 // Rule A depends on Lemma B
	RelGeneralizes  RelationType = 2 // Rule A generalizes or covers residue class of B
	RelComposition  RelationType = 3 // Rule C is derived from composing A and B
)

// GraphEdge represents a directed relationship between mathematical rules.
type GraphEdge struct {
	Target int
	Rel    RelationType
}

// MathCSRGraph implements a Compressed Sparse Row (CSR) structure for rule relationships.
type MathCSRGraph struct {
	RuleIDs    []string
	IDToIdx    map[string]int
	RowOffsets []int
	ColIndices []int
	RelTypes   []RelationType
}

// NewMathCSRGraph constructs a CSR graph from a slice of rules.
func NewMathCSRGraph(rules []*Rule) *MathCSRGraph {
	n := len(rules)
	idToIdx := make(map[string]int, n)
	ruleIDs := make([]string, n)
	for i, r := range rules {
		idToIdx[r.ID] = i
		ruleIDs[i] = r.ID
	}

	// First pass: collect adjacency lists
	adj := make([][]GraphEdge, n)
	for i, r := range rules {
		for _, depID := range r.Dependencies {
			if depIdx, ok := idToIdx[depID]; ok {
				adj[i] = append(adj[i], GraphEdge{Target: depIdx, Rel: RelDependency})
			}
		}
	}

	// Detect residue subsumptions (e.g. MOD24-17 specializes MOD4-1 etc.)
	for i, r1 := range rules {
		if r1.Modulus <= 0 {
			continue
		}
		for j, r2 := range rules {
			if i == j || r2.Modulus <= 0 {
				continue
			}
			if r1.Modulus%r2.Modulus == 0 {
				// Check if r1 residues are contained in r2 mod r2.Modulus
				subsumed := true
				for _, res1 := range r1.Residues {
					m2res := res1 % r2.Modulus
					if !slices.Contains(r2.Residues, m2res) {
						subsumed = false
						break
					}
				}
				if subsumed && len(r1.Residues) > 0 {
					adj[i] = append(adj[i], GraphEdge{Target: j, Rel: RelGeneralizes})
				}
			}
		}
	}

	// Pack into CSR representation
	totalEdges := 0
	for _, edges := range adj {
		totalEdges += len(edges)
	}

	rowOffsets := make([]int, n+1)
	colIndices := make([]int, totalEdges)
	relTypes := make([]RelationType, totalEdges)

	edgeIdx := 0
	for i := 0; i < n; i++ {
		rowOffsets[i] = edgeIdx
		for _, e := range adj[i] {
			colIndices[edgeIdx] = e.Target
			relTypes[edgeIdx] = e.Rel
			edgeIdx++
		}
	}
	rowOffsets[n] = totalEdges

	return &MathCSRGraph{
		RuleIDs:    ruleIDs,
		IDToIdx:    idToIdx,
		RowOffsets: rowOffsets,
		ColIndices: colIndices,
		RelTypes:   relTypes,
	}
}

// Neighbors returns outgoing neighbors and their relation types for a given rule index.
func (g *MathCSRGraph) Neighbors(ruleIdx int) []GraphEdge {
	if ruleIdx < 0 || ruleIdx >= len(g.RowOffsets)-1 {
		return nil
	}
	start := g.RowOffsets[ruleIdx]
	end := g.RowOffsets[ruleIdx+1]
	edges := make([]GraphEdge, 0, end-start)
	for i := start; i < end; i++ {
		edges = append(edges, GraphEdge{
			Target: g.ColIndices[i],
			Rel:    g.RelTypes[i],
		})
	}
	return edges
}

// AddEdge dynamically appends a new edge (used during composition).
func (g *MathCSRGraph) AddEdge(sourceID, targetID string, rel RelationType) error {
	src, okSrc := g.IDToIdx[sourceID]
	tgt, okTgt := g.IDToIdx[targetID]
	if !okSrc || !okTgt {
		return fmt.Errorf("cannot link %s -> %s: %w", sourceID, targetID, ErrInvalidInput)
	}

	// Reconstruct adjacency, add edge, repack
	n := len(g.RuleIDs)
	adj := make([][]GraphEdge, n)
	for i := 0; i < n; i++ {
		adj[i] = g.Neighbors(i)
	}
	adj[src] = append(adj[src], GraphEdge{Target: tgt, Rel: rel})

	totalEdges := 0
	for _, edges := range adj {
		totalEdges += len(edges)
	}
	rowOffsets := make([]int, n+1)
	colIndices := make([]int, totalEdges)
	relTypes := make([]RelationType, totalEdges)

	edgeIdx := 0
	for i := 0; i < n; i++ {
		rowOffsets[i] = edgeIdx
		for _, e := range adj[i] {
			colIndices[edgeIdx] = e.Target
			relTypes[edgeIdx] = e.Rel
			edgeIdx++
		}
	}
	rowOffsets[n] = totalEdges

	g.RowOffsets = rowOffsets
	g.ColIndices = colIndices
	g.RelTypes = relTypes
	return nil
}
