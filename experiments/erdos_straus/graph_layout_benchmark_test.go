package erdosstraus

import (
	"fmt"
	"testing"
	"unsafe"
)

// graphLayoutFixture contains identical synthetic directed edges encoded as
// a flat production MathCSRGraph and as conventional [][]GraphEdge.
// No solver, modular rule index or model inference is part of this fixture.
type graphLayoutFixture struct {
	csr *MathCSRGraph
	adj [][]GraphEdge
	nodes int
	degree int
	csrBytes uint64
	listBytes uint64
}

func newGraphLayoutFixture(nodes, degree int) graphLayoutFixture {
	edges := nodes * degree
	c := &MathCSRGraph{
		RowOffsets: make([]int, nodes+1),
		ColIndices: make([]int, edges),
		RelTypes: make([]RelationType, edges),
	}
	a := make([][]GraphEdge, nodes)
	for i := 0; i < nodes; i++ {
		c.RowOffsets[i] = i * degree
		a[i] = make([]GraphEdge, degree)
		for j := 0; j < degree; j++ {
			at := i*degree+j
			e := GraphEdge{
				Target: (i*131 + j*977 + (i >> 2)) % nodes,
				Rel: RelationType(j%3+1),
			}
			a[i][j] = e
			c.ColIndices[at] = e.Target
			c.RelTypes[at] = e.Rel
		}
	}
	c.RowOffsets[nodes] = edges
	csrBytes := uint64(cap(c.RowOffsets))*uint64(unsafe.Sizeof(int(0))) +
		uint64(cap(c.ColIndices))*uint64(unsafe.Sizeof(int(0))) +
		uint64(cap(c.RelTypes))*uint64(unsafe.Sizeof(RelationType(0)))
	listBytes := uint64(cap(a))*uint64(unsafe.Sizeof([]GraphEdge(nil)))
	for _, row := range a {
		listBytes += uint64(cap(row)) * uint64(unsafe.Sizeof(GraphEdge{}))
	}
	return graphLayoutFixture{csr:c, adj:a, nodes:nodes, degree:degree,
		csrBytes:csrBytes, listBytes:listBytes}
}

func graphLayoutScanCSRRaw(g *MathCSRGraph, node int) uint64 {
	var sum uint64
	for i := g.RowOffsets[node]; i < g.RowOffsets[node+1]; i++ {
		sum += uint64(g.ColIndices[i])*7 + uint64(g.RelTypes[i])
	}
	return sum
}

func graphLayoutScanCSRPublic(g *MathCSRGraph, node int) uint64 {
	var sum uint64
	for _, e := range g.Neighbors(node) {
		sum += uint64(e.Target)*7 + uint64(e.Rel)
	}
	return sum
}

func graphLayoutScanAdj(row []GraphEdge) uint64 {
	var sum uint64
	for _, e := range row {
		sum += uint64(e.Target)*7 + uint64(e.Rel)
	}
	return sum
}

func TestGraphLayoutEquivalentEdgesAndStructuralBytes(t *testing.T) {
	for _, size := range []struct{nodes, degree int}{
		{128,4}, {1000,10}, {10000,10},
	} {
		f := newGraphLayoutFixture(size.nodes, size.degree)
		if len(f.csr.RowOffsets) != size.nodes+1 ||
			len(f.csr.ColIndices) != size.nodes*size.degree {
			t.Fatalf("invalid CSR size: %+v", size)
		}
		if f.csrBytes >= f.listBytes {
			t.Fatalf("unexpected structural sizes for %+v: csr=%d list=%d", size, f.csrBytes, f.listBytes)
		}
		for node := 0; node < f.nodes; node++ {
			want := graphLayoutScanAdj(f.adj[node])
			if got := graphLayoutScanCSRRaw(f.csr,node); got != want {
				t.Fatalf("raw CSR differs on node %d: %d vs %d", node, got, want)
			}
			if got := graphLayoutScanCSRPublic(f.csr,node); got != want {
				t.Fatalf("public CSR differs on node %d: %d vs %d", node, got, want)
			}
		}
	}
}

var graphLayoutBenchmarkSink uint64

// BenchmarkGraphLayoutRowScan measures one adjacency-row traversal per op.
// The fixture and query stream are shared. Raw arrays (not Neighbors) are
// deliberately measured separately from the actual public allocating API.
// Structural capacity bytes are NOT live heap allocation measurements.
func BenchmarkGraphLayoutRowScan(b *testing.B) {
	for _, size := range []struct{nodes, degree int}{
		{1000,10}, {10000,10}, {50000,20},
	} {
		b.Run(fmt.Sprintf("nodes=%d_edges=%d",size.nodes,size.nodes*size.degree),func(b *testing.B) {
			f := newGraphLayoutFixture(size.nodes,size.degree)
			starts := make([]int,1024)
			for i := range starts { starts[i] = (i*7919)%size.nodes }
			variants := []struct{name string;scan func(int)uint64}{
				{"AdjacencyList",func(node int)uint64{return graphLayoutScanAdj(f.adj[node])}},
				{"CSRRawArrays",func(node int)uint64{return graphLayoutScanCSRRaw(f.csr,node)}},
				{"CSRPublicNeighbors",func(node int)uint64{return graphLayoutScanCSRPublic(f.csr,node)}},
			}
			for _, variant := range variants {
				b.Run(variant.name,func(b *testing.B) {
					b.ReportAllocs()
					var sum uint64
					b.ResetTimer()
					for i:=0;i<b.N;i++ { sum += variant.scan(starts[i&1023]) }
					graphLayoutBenchmarkSink=sum
					b.ReportMetric(float64(f.csrBytes),"csr-struct-B")
					b.ReportMetric(float64(f.listBytes),"list-struct-B")
				})
			}
		})
	}
}
