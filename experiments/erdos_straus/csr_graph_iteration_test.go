package erdosstraus

import "testing"

func TestCSRForEachNeighborParityAndEarlyStop(t *testing.T) {
	g := &MathCSRGraph{
		RowOffsets: []int{0, 3, 4, 4},
		ColIndices: []int{2, 1, 0, 2},
		RelTypes:   []RelationType{RelDependency, RelGeneralizes, RelComposition, RelDependency},
	}
	for row := 0; row < 3; row++ {
		want := g.Neighbors(row)
		got := make([]GraphEdge, 0, len(want))
		g.ForEachNeighbor(row, func(target int, relation RelationType) bool {
			got = append(got, GraphEdge{Target: target, Rel: relation})
			return true
		})
		if len(got) != len(want) {
			t.Fatalf("row %d: got %d edges, want %d", row, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("row %d edge %d: got %+v, want %+v", row, i, got[i], want[i])
			}
		}
	}
	count := 0
	g.ForEachNeighbor(0, func(int, RelationType) bool {
		count++
		return false
	})
	if count != 1 {
		t.Fatalf("early-stop visited %d edges, want 1", count)
	}
	for _, invalid := range []int{-1, 3, 100} {
		g.ForEachNeighbor(invalid, func(int, RelationType) bool {
			t.Fatalf("invalid rule index %d yielded an edge", invalid)
			return false
		})
	}
}

var csrIterationTestSink uint64

func TestCSRForEachNeighborZeroAllocations(t *testing.T) {
	// The non-empty row makes allocation regressions visible.
	g := &MathCSRGraph{
		RowOffsets: []int{0, 10},
		ColIndices: []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
		RelTypes: []RelationType{
			RelDependency, RelComposition, RelGeneralizes, RelDependency,
			RelComposition, RelGeneralizes, RelDependency, RelComposition,
			RelGeneralizes, RelDependency,
		},
	}
	var sum uint64
	allocs := testing.AllocsPerRun(1000, func() {
		g.ForEachNeighbor(0, func(target int, rel RelationType) bool {
			sum += uint64(target)*7 + uint64(rel)
			return true
		})
	})
	csrIterationTestSink = sum
	if allocs != 0 {
		t.Fatalf("ForEachNeighbor allocated %.2f times per 10-edge row; want 0", allocs)
	}
}
