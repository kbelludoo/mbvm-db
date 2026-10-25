package erdosstraus

// ForEachNeighbor visits a rule's outgoing CSR edges in their stored order.
//
// Unlike Neighbors, it does not materialize a temporary []GraphEdge. Returning
// false from visit stops iteration immediately; true visits the next edge.
// An invalid rule index behaves like Neighbors: it visits no edges.
//
// Callers must not mutate the graph during iteration. As with the existing
// graph implementation, concurrent mutation and iteration require external
// synchronization. The callback must not retain assumptions about edge
// offsets across graph updates.
func (g *MathCSRGraph) ForEachNeighbor(ruleIdx int, visit func(target int, rel RelationType) bool) {
	if ruleIdx < 0 || ruleIdx >= len(g.RowOffsets)-1 {
		return
	}
	for edge := g.RowOffsets[ruleIdx]; edge < g.RowOffsets[ruleIdx+1]; edge++ {
		if !visit(g.ColIndices[edge], g.RelTypes[edge]) {
			return
		}
	}
}
