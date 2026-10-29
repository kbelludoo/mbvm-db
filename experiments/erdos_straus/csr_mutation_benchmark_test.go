package erdosstraus

import (
	"fmt"
	"strconv"
	"testing"
)

// Identical input graph for both insertion strategies. Construction is
// excluded from the AddEdge/AddEdges benchmark: only CSR update cost is timed.
func csrMutationFixture(nodes, degree int) *MathCSRGraph {
	graph := newGraphLayoutFixture(nodes, degree).csr
	graph.RuleIDs = make([]string, nodes)
	graph.IDToIdx = make(map[string]int, nodes)
	for node := 0; node < nodes; node++ {
		id := "node-" + strconv.Itoa(node)
		graph.RuleIDs[node] = id
		graph.IDToIdx[id] = node
	}
	return graph
}

func csrMutationBatch(graph *MathCSRGraph, count int) []CSRNamedEdge {
	edges := make([]CSRNamedEdge, count)
	for i := range edges {
		edges[i] = CSRNamedEdge{
			SourceID: graph.RuleIDs[(i*131+7)%len(graph.RuleIDs)],
			TargetID: graph.RuleIDs[(i*977+11)%len(graph.RuleIDs)],
			Rel:      RelComposition,
		}
	}
	return edges
}

// BenchmarkCSRMutationAddEdges compares identical complete update requests
// using repeated legacy AddEdge calls versus one batch. Copying the graph
// header per round is common to both arms. Existing arrays are never mutated,
// so a shallow header copy is sufficient for the input snapshot.
func BenchmarkCSRMutationAddEdges(b *testing.B) {
	for _, size := range []struct{nodes, degree, insertions int}{
		{1000, 10, 32},
		{10000, 10, 32},
	} {
		b.Run(fmt.Sprintf("nodes=%d_existing_edges=%d_insertions=%d",
			size.nodes, size.nodes*size.degree, size.insertions), func(b *testing.B) {
			seed := csrMutationFixture(size.nodes, size.degree)
			batch := csrMutationBatch(seed, size.insertions)
			for _, mode := range []struct{name string; oneByOne bool}{
				{"RepeatedAddEdge", true},
				{"SingleAddEdgesBatch", false},
			} {
				b.Run(mode.name, func(b *testing.B) {
					b.ReportAllocs()
					var checksum int
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						next := *seed
						if mode.oneByOne {
							for _, e := range batch {
								if err := next.AddEdge(e.SourceID, e.TargetID, e.Rel); err != nil {
									b.Fatal(err)
								}
							}
						} else {
							if err := next.AddEdges(batch); err != nil {
								b.Fatal(err)
							}
						}
						checksum += len(next.ColIndices)
					}
					graphLayoutBenchmarkSink = uint64(checksum)
				})
			}
		})
	}
}

// BenchmarkCSRBuildModularPairs isolates the quadratic residue-subsumption
// scan in NewMathCSRGraph from dependency-only graph construction.
// All modular residues are distinct: no generalization edges are produced,
// but the pairwise checks are still needed by the current constructor.
func BenchmarkCSRBuildModularPairs(b *testing.B) {
	for _, nodes := range []int{256, 1024, 2048} {
		for _, modular := range []bool{false, true} {
			label := "DependencyOnly"
			if modular { label = "DistinctModularResidues" }
			b.Run(fmt.Sprintf("nodes=%d/%s", nodes, label), func(b *testing.B) {
				rules := make([]*Rule, nodes)
				for i := range rules {
					r := &Rule{ID: "rule-" + strconv.Itoa(i)}
					if modular {
						r.Modulus = 1000003
						r.Residues = []int64{int64(i)}
					}
					rules[i] = r
				}
				b.ReportAllocs()
				var total int
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					graph := NewMathCSRGraph(rules)
					total += len(graph.RowOffsets) + len(graph.ColIndices)
				}
				graphLayoutBenchmarkSink = uint64(total)
			})
		}
	}
}
