package erdosstraus

import (
	"fmt"
	"testing"
)

// Same frontier, hop limit, relation checksum, and visited-set semantics for
// every representation. Mark buffers/queues are reused to isolate traversal
// cost instead of allocating a new visited map on every operation.
type graphLayoutVisit struct {
	node int
	depth uint8
}

func graphLayoutWalkAdj(adj [][]GraphEdge, root int, stamp uint32,
	seen []uint32, queue []graphLayoutVisit) uint64 {
	queue=append(queue[:0],graphLayoutVisit{node:root})
	seen[root]=stamp
	var sum uint64
	for i:=0;i<len(queue);i++ {
		current:=queue[i]
		if current.depth>=2 {continue}
		for _,e:=range adj[current.node] {
			sum+=uint64(e.Target)*7+uint64(e.Rel)
			if seen[e.Target]!=stamp {
				seen[e.Target]=stamp
				queue=append(queue,graphLayoutVisit{node:e.Target,depth:current.depth+1})
			}
		}
	}
	return sum
}

func graphLayoutWalkCSRRaw(g *MathCSRGraph, root int, stamp uint32,
	seen []uint32, queue []graphLayoutVisit) uint64 {
	queue=append(queue[:0],graphLayoutVisit{node:root})
	seen[root]=stamp
	var sum uint64
	for i:=0;i<len(queue);i++ {
		current:=queue[i]
		if current.depth>=2 {continue}
		for e:=g.RowOffsets[current.node];e<g.RowOffsets[current.node+1];e++ {
			target,relation:=g.ColIndices[e],g.RelTypes[e]
			sum+=uint64(target)*7+uint64(relation)
			if seen[target]!=stamp {
				seen[target]=stamp
				queue=append(queue,graphLayoutVisit{node:target,depth:current.depth+1})
			}
		}
	}
	return sum
}

func graphLayoutWalkCSRPublic(g *MathCSRGraph, root int, stamp uint32,
	seen []uint32, queue []graphLayoutVisit) uint64 {
	queue=append(queue[:0],graphLayoutVisit{node:root})
	seen[root]=stamp
	var sum uint64
	for i:=0;i<len(queue);i++ {
		current:=queue[i]
		if current.depth>=2 {continue}
		for _,e:=range g.Neighbors(current.node) {
			sum+=uint64(e.Target)*7+uint64(e.Rel)
			if seen[e.Target]!=stamp {
				seen[e.Target]=stamp
				queue=append(queue,graphLayoutVisit{node:e.Target,depth:current.depth+1})
			}
		}
	}
	return sum
}

func TestGraphLayoutTwoHopParity(t *testing.T) {
	for _,size:=range []struct{nodes,degree int}{{128,4},{1000,10},{10000,10}} {
		f:=newGraphLayoutFixture(size.nodes,size.degree)
		seen:=make([]uint32,f.nodes)
		queue:=make([]graphLayoutVisit,0,f.nodes)
		for j,root:=range []int{0,1,17,f.nodes/2,f.nodes-1} {
			stamp:=uint32(3*j+1)
			want:=graphLayoutWalkAdj(f.adj,root,stamp,seen,queue)
			if got:=graphLayoutWalkCSRRaw(f.csr,root,stamp+1,seen,queue);got!=want {
				t.Fatalf("two-hop raw CSR diverges at %d/%d: got %d want %d",size.nodes,root,got,want)
			}
			if got:=graphLayoutWalkCSRPublic(f.csr,root,stamp+2,seen,queue);got!=want {
				t.Fatalf("two-hop public CSR diverges at %d/%d: got %d want %d",size.nodes,root,got,want)
			}
		}
	}
}

// BenchmarkGraphLayoutTwoHop measures complete two-hop graph traversals rather
// than only a single row scan. One operation uses a deterministic query root.
// Do not interpret this synthetic, warmed benchmark as hardware cache profiling.
func BenchmarkGraphLayoutTwoHop(b *testing.B) {
	for _,size:=range []struct{nodes,degree int}{
		{1000,10},{10000,10},{50000,20},
	} {
		b.Run(fmt.Sprintf("nodes=%d_edges=%d",size.nodes,size.nodes*size.degree),func(b *testing.B) {
			f:=newGraphLayoutFixture(size.nodes,size.degree)
			roots:=make([]int,1024)
			for i:=range roots{roots[i]=(i*7919)%size.nodes}
			variants:=[]struct{
				name string
				visit func(root int,stamp uint32,seen []uint32,queue []graphLayoutVisit)uint64
			}{
				{"AdjacencyList",func(root int,stamp uint32,seen []uint32,queue []graphLayoutVisit)uint64{
					return graphLayoutWalkAdj(f.adj,root,stamp,seen,queue)
				}},
				{"CSRRawArrays",func(root int,stamp uint32,seen []uint32,queue []graphLayoutVisit)uint64{
					return graphLayoutWalkCSRRaw(f.csr,root,stamp,seen,queue)
				}},
				{"CSRPublicNeighbors",func(root int,stamp uint32,seen []uint32,queue []graphLayoutVisit)uint64{
					return graphLayoutWalkCSRPublic(f.csr,root,stamp,seen,queue)
				}},
			}
			for _,v:=range variants {
				b.Run(v.name,func(b *testing.B) {
					seen:=make([]uint32,f.nodes)
					queue:=make([]graphLayoutVisit,0,f.nodes)
					b.ReportAllocs()
					var sum uint64
					b.ResetTimer()
					for i:=0;i<b.N;i++ {
						sum+=v.visit(roots[i&1023],uint32(i+1),seen,queue)
					}
					graphLayoutBenchmarkSink=sum
					b.ReportMetric(float64(f.csrBytes),"csr-struct-B")
					b.ReportMetric(float64(f.listBytes),"list-struct-B")
				})
			}
		})
	}
}
