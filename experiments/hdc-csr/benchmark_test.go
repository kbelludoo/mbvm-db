package hdccsr

import (
	"math/rand"
	"testing"
)

func BenchmarkHammingDistance8K(b *testing.B) {
	rng := rand.New(rand.NewSource(1))
	v1 := generateRandomHypervector(rng)
	v2 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	sink := 0
	for i := 0; i < b.N; i++ {
		sink += HammingDistance(v1, v2)
	}
	_ = sink
}

func BenchmarkBind8K(b *testing.B) {
	rng := rand.New(rand.NewSource(2))
	v1 := generateRandomHypervector(rng)
	v2 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink = Bind(v1, v2)
	}
	_ = sink
}

func BenchmarkRotL8K(b *testing.B) {
	rng := rand.New(rand.NewSource(3))
	v1 := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink = RotL8K(v1, 13)
	}
	_ = sink
}

func BenchmarkMajority3Vectors8K(b *testing.B) {
	rng := rand.New(rand.NewSource(4))
	msgs := []Hypervector{
		generateRandomHypervector(rng),
		generateRandomHypervector(rng),
		generateRandomHypervector(rng),
	}
	mask := generateRandomHypervector(rng)

	b.ResetTimer()
	b.ReportAllocs()

	var sink Hypervector
	for i := 0; i < b.N; i++ {
		sink, _ = Majority(msgs, mask)
	}
	_ = sink
}

func BenchmarkPropagateHop1_1000Nodes(b *testing.B) {
	const totalNodes = 1000
	rng := rand.New(rand.NewSource(5))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = generateRandomHypervector(rng)
	}

	// Constrói grafo esparso (média de 4 arestas por nó = 4.000 arestas)
	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 4; k++ {
			target := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: target, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = hdc.PropagateHop1(graph, orig)
	}
}


// These benchmarks isolate graph-building costs from query costs.
// The HDC full scan visits every candidate: it does NOT provide global O(1) retrieval.
var benchmarkContextSink *ContextualMemory
var benchmarkScoreSink int

func BenchmarkPropagateHop2_1000Nodes(b *testing.B) {
    f := buildMultipathFixture(8080,1000)
    sys := NewHDCSystem(42)
    hop1 := sys.PropagateHop1(f.graph,f.vectors)
    b.ReportAllocs()
    b.ResetTimer()
    for i:=0;i<b.N;i++ {
        benchmarkContextSink=sys.PropagateHop2(f.graph,hop1)
    }
}

func BenchmarkCSR2HopSemantics_1000Nodes(b *testing.B) {
    f := buildMultipathFixture(8080,1000)
    target:=f.prototypes[1]
    sum:=0
    b.ReportAllocs()
    b.ResetTimer()
    for i:=0;i<b.N;i++ {
        src:=i%1000
        ids:=csrTwoHopCallTargets(f.graph,src)
        // Exact CSR candidate set plus a semantic filter.
        for _,id:=range ids {
            if HammingDistance(target,f.vectors[id]) < 1500 { sum++ }
        }
    }
    benchmarkScoreSink=sum
}

func BenchmarkHDC2HopMembership_1000Nodes(b *testing.B) {
    f:=buildMultipathFixture(8080,1000)
    sys:=NewHDCSystem(42)
    hop2:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
    cfg:=sys.RelConfigs[RelCall]
    role:=Bind(RotL8K(cfg.RoleVector,cfg.Shift),cfg.RoleVector)
    sum:=0
    b.ReportAllocs()
    b.ResetTimer()
    for i:=0;i<b.N;i++ {
        src:=i%1000
        recovered:=RotR8K(Unbind(hop2.Contextual[src],role),cfg.Shift*2)
        sum+=HammingDistance(recovered,f.vectors[(src+37)%1000])
    }
    benchmarkScoreSink=sum
}

func BenchmarkHDC2HopFullScan_1000Nodes(b *testing.B) {
    f:=buildMultipathFixture(8080,1000)
    sys:=NewHDCSystem(42)
    hop2:=sys.PropagateHop2(f.graph,sys.PropagateHop1(f.graph,f.vectors))
    cfg:=sys.RelConfigs[RelCall]
    role:=Bind(RotL8K(cfg.RoleVector,cfg.Shift),cfg.RoleVector)
    sum:=0
    b.ReportAllocs()
    b.ResetTimer()
    for i:=0;i<b.N;i++ {
        src:=i%1000
        recovered:=RotR8K(Unbind(hop2.Contextual[src],role),cfg.Shift*2)
        best:=VectorBits+1
        for id:=0;id<1000;id++ {
            if id==src { continue }
            d:=HammingDistance(recovered,f.vectors[id])
            if d<best { best=d }
        }
        sum+=best
    }
    benchmarkScoreSink=sum
}
