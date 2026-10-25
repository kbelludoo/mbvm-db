package erdosstraus

import "fmt"

// CSRNamedEdge identifies a directed graph relation using stable rule IDs.
// AddEdges keeps the order of appended edges within each source row.
type CSRNamedEdge struct {
	SourceID string
	TargetID string
	Rel      RelationType
}

// AddEdges appends an entire batch with one CSR rebuild: O(V + E + B)
// work and O(V + E + B) temporary storage for V nodes, E existing edges
// and B new edges. In contrast, repeating AddEdge B times rebuilds B times.
//
// All IDs and existing CSR offsets are checked before any graph field is
// modified. If validation fails the graph remains unchanged. Duplicates
// are preserved, matching repeated AddEdge calls.
//
// Concurrent graph readers and writers still require external locking.
func (g *MathCSRGraph) AddEdges(batch []CSRNamedEdge) error {
	if len(batch) == 0 {
		return nil
	}
	nodes := len(g.RuleIDs)
	existing := len(g.ColIndices)
	if len(g.RowOffsets) != nodes+1 || len(g.RelTypes) != existing ||
		g.RowOffsets[0] != 0 || g.RowOffsets[nodes] != existing {
		return fmt.Errorf("malformed CSR arrays: %w", ErrInvalidInput)
	}

	counts := make([]int, nodes)
	sources := make([]int, len(batch))
	targets := make([]int, len(batch))
	for i, edge := range batch {
		source, sourceOK := g.IDToIdx[edge.SourceID]
		target, targetOK := g.IDToIdx[edge.TargetID]
		if !sourceOK || !targetOK || source < 0 || source >= nodes ||
			target < 0 || target >= nodes {
			return fmt.Errorf("unknown CSR edge %s -> %s: %w",
				edge.SourceID, edge.TargetID, ErrInvalidInput)
		}
		sources[i], targets[i] = source, target
		counts[source]++
	}

	offsets := make([]int, nodes+1)
	for node := 0; node < nodes; node++ {
		lo, hi := g.RowOffsets[node], g.RowOffsets[node+1]
		if lo < 0 || hi < lo || hi > existing {
			return fmt.Errorf("invalid CSR row %d offsets [%d,%d): %w",
				node, lo, hi, ErrInvalidInput)
		}
		offsets[node+1] = offsets[node] + hi-lo + counts[node]
	}

	columns := make([]int, offsets[nodes])
	relations := make([]RelationType, offsets[nodes])
	cursors := make([]int, nodes)
	for node := 0; node < nodes; node++ {
		start, end := g.RowOffsets[node], g.RowOffsets[node+1]
		destination := offsets[node]
		copy(columns[destination:], g.ColIndices[start:end])
		copy(relations[destination:], g.RelTypes[start:end])
		cursors[node] = destination + end-start
	}
	for i, edge := range batch {
		source := sources[i]
		at := cursors[source]
		columns[at] = targets[i]
		relations[at] = edge.Rel
		cursors[source]++
	}

	g.RowOffsets = offsets
	g.ColIndices = columns
	g.RelTypes = relations
	return nil
}
