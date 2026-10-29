package erdosstraus

import (
	"slices"
	"testing"
)

func csrBatchTestGraph() *MathCSRGraph {
	return &MathCSRGraph{
		RuleIDs:    []string{"A", "B", "C", "D"},
		IDToIdx:    map[string]int{"A": 0, "B": 1, "C": 2, "D": 3},
		RowOffsets: []int{0, 1, 2, 2, 2},
		ColIndices: []int{1, 2},
		RelTypes:   []RelationType{RelDependency, RelGeneralizes},
	}
}
func csrBatchClone(g *MathCSRGraph) *MathCSRGraph {
	c := *g
	c.RowOffsets = slices.Clone(g.RowOffsets)
	c.ColIndices = slices.Clone(g.ColIndices)
	c.RelTypes = slices.Clone(g.RelTypes)
	return &c
}

func TestCSRAddEdgesEqualsSequentialWithInterleavedSources(t *testing.T) {
	batch := []CSRNamedEdge{
		{SourceID: "A", TargetID: "C", Rel: RelComposition},
		{SourceID: "D", TargetID: "A", Rel: RelDependency},
		{SourceID: "B", TargetID: "D", Rel: RelComposition},
		{SourceID: "A", TargetID: "D", Rel: RelGeneralizes},
		{SourceID: "A", TargetID: "C", Rel: RelComposition}, // duplicates are allowed
	}
	oneByOne := csrBatchTestGraph()
	for _, e := range batch {
		if err := oneByOne.AddEdge(e.SourceID, e.TargetID, e.Rel); err != nil {
			t.Fatalf("sequential AddEdge failed: %v", err)
		}
	}
	allAtOnce := csrBatchTestGraph()
	if err := allAtOnce.AddEdges(batch); err != nil {
		t.Fatalf("batched AddEdges failed: %v", err)
	}
	if !slices.Equal(oneByOne.RowOffsets, allAtOnce.RowOffsets) ||
		!slices.Equal(oneByOne.ColIndices, allAtOnce.ColIndices) ||
		!slices.Equal(oneByOne.RelTypes, allAtOnce.RelTypes) {
		t.Fatalf("batch changed edge order or relation types:\nsequential=%+v\nbatch=%+v", oneByOne, allAtOnce)
	}
	// Independent expected layout: tests against the intended edge sequence,
	// not merely two code paths sharing the same AddEdges implementation.
	if !slices.Equal(allAtOnce.RowOffsets, []int{0, 4, 6, 6, 7}) ||
		!slices.Equal(allAtOnce.ColIndices, []int{1, 2, 3, 2, 2, 3, 0}) ||
		!slices.Equal(allAtOnce.RelTypes, []RelationType{
			RelDependency, RelComposition, RelGeneralizes, RelComposition,
			RelGeneralizes, RelComposition, RelDependency,
		}) {
		t.Fatalf("batch has incorrect independently expected CSR arrays: %+v", allAtOnce)
	}
	for row := range allAtOnce.RuleIDs {
		want := oneByOne.Neighbors(row)
		got := allAtOnce.Neighbors(row)
		if !slices.Equal(got, want) {
			t.Fatalf("row %d differs: got=%v want=%v", row, got, want)
		}
	}
}

func TestCSRAddEdgesAtomicValidation(t *testing.T) {
	g := csrBatchTestGraph()
	original := csrBatchClone(g)
	err := g.AddEdges([]CSRNamedEdge{
		{SourceID: "A", TargetID: "C", Rel: RelComposition},
		{SourceID: "DOES-NOT-EXIST", TargetID: "B", Rel: RelDependency},
	})
	if err == nil {
		t.Fatal("invalid ID should reject whole batch")
	}
	if !slices.Equal(g.RowOffsets, original.RowOffsets) ||
		!slices.Equal(g.ColIndices, original.ColIndices) ||
		!slices.Equal(g.RelTypes, original.RelTypes) {
		t.Fatalf("partial update escaped after invalid batch")
	}
	if err := g.AddEdges(nil); err != nil {
		t.Fatalf("empty AddEdges batch failed: %v", err)
	}
	if !slices.Equal(g.RowOffsets, original.RowOffsets) {
		t.Fatal("empty batch modified offsets")
	}
	g.RowOffsets[2] = -1
	before := csrBatchClone(g)
	if err := g.AddEdges([]CSRNamedEdge{{SourceID: "D", TargetID: "A", Rel: RelComposition}}); err == nil {
		t.Fatal("corrupted offsets must reject update")
	}
	if !slices.Equal(g.RowOffsets, before.RowOffsets) ||
		!slices.Equal(g.ColIndices, before.ColIndices) ||
		!slices.Equal(g.RelTypes, before.RelTypes) {
		t.Fatal("corrupted layout validation changed graph")
	}
}
