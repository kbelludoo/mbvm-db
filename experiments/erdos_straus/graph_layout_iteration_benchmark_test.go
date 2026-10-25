package erdosstraus

import (
	"fmt"
	"testing"
)

// Exercise the new public allocation-free traversal, alongside the existing
// allocating public API and the direct-array reference implementation.
func graphLayoutScanCSRForEach(g *MathCSRGraph, node int) uint64 {
	var sum uint64
	g.ForEachNeighbor(node, func(target int, rel RelationType) bool {
		sum += uint64(target)*7 + uint64(rel)
		return true
	})
	return sum
}

func graphLayoutWalkCSRForEach(g *MathCSRGraph, root int, stamp uint32,
	seen []uint32, queue []graphLayoutVisit) uint64 {
	queue = append(queue[:0], graphLayoutVisit{node: root})
	seen[root] = stamp
	var sum uint64
	for i := 0; i < len(queue); i++ {
		current := queue[i]
		if current.depth >= 2 {
			continue
		}
		g.ForEachNeighbor(current.node, func(target int, rel RelationType) bool {
			sum += uint64(target)*7 + uint64(rel)
			if seen[target] != stamp {
				seen[target] = stamp
				queue = append(queue, graphLayoutVisit{node: target, depth: current.depth+1})
			}
			return true
		})
	}
	return sum
}

func TestGraphLayoutCSRForEachParity(t *testing.T) {
	for _, spec := range []struct{ nodes, degree int }{
		{128, 4}, {1000, 10}, {10000, 10},
	} {
		f := newGraphLayoutFixture(spec.nodes, spec.degree)
		seen := make([]uint32, f.nodes)
		queue := make([]graphLayoutVisit, 0, f.nodes)
		for j, root := range []int{0, 1, 17, f.nodes / 2, f.nodes - 1} {
			if got, want := graphLayoutScanCSRForEach(f.csr, root), graphLayoutScanAdj(f.adj[root]); got != want {
				t.Fatalf("row scan (%d nodes) root %d: got %d, want %d", f.nodes, root, got, want)
			}
			stamp := uint32(3*j + 1)
			want := graphLayoutWalkAdj(f.adj, root, stamp, seen, queue)
			got := graphLayoutWalkCSRForEach(f.csr, root, stamp+1, seen, queue)
			if got != want {
				t.Fatalf("two-hop (%d nodes) root %d: got %d, want %d", f.nodes, root, got, want)
			}
		}
	}
}

// BenchmarkGraphLayoutForEach has the same graph fixtures and root streams as
// BenchmarkGraphLayoutRowScan/TwoHop in the preceding PR. ReportAllocs is
// required because closure escape behavior depends on the Go toolchain.
func BenchmarkGraphLayoutForEach(b *testing.B) {
	for _, spec := range []struct{ nodes, degree int }{
		{1000, 10}, {10000, 10}, {50000, 20},
	} {
		b.Run(fmt.Sprintf("nodes=%d_edges=%d", spec.nodes, spec.nodes*spec.degree), func(b *testing.B) {
			f := newGraphLayoutFixture(spec.nodes, spec.degree)
			roots := make([]int, 1024)
			for i := range roots {
				roots[i] = (i*7919)%f.nodes
			}
			b.Run("RowScan", func(b *testing.B) {
				b.ReportAllocs()
				var sum uint64
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sum += graphLayoutScanCSRForEach(f.csr, roots[i&1023])
				}
				graphLayoutBenchmarkSink = sum
			})
			b.Run("TwoHop", func(b *testing.B) {
				seen := make([]uint32, f.nodes)
				queue := make([]graphLayoutVisit, 0, f.nodes)
				b.ReportAllocs()
				var sum uint64
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					sum += graphLayoutWalkCSRForEach(f.csr, roots[i&1023], uint32(i+1), seen, queue)
				}
				graphLayoutBenchmarkSink = sum
			})
		})
	}
}
