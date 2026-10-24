package hdccsr

import (
	"math/rand"
	"testing"
	"time"
)

func TestIncremental2HopByteForByteExactness(t *testing.T) {
	const totalNodes = 100
	rng := rand.New(rand.NewSource(12345))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	// Constrói grafo com topologia não-trivial (3 arestas por nó)
	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 1; k <= 3; k++ {
			target := uint16((i*7 + k*13) % totalNodes)
			if int(target) != i {
				adj[i] = append(adj[i], AdjEdge{Target: target, Rel: RelCall})
			}
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(999)

	// 1. Constrói a memória de 2-hop inicial completa
	hop1Init := hdc.PropagateHop1(graph, orig)
	memInit := hdc.PropagateHop2(graph, hop1Init)

	// Escolhe um nó para sofrer mutação (ex: Nó 42)
	mutatedNode := uint16(42)
	newVec := GenerateRandomHypervector(rand.New(rand.NewSource(8888)))

	// 2. Caminho A: Reconstrução Completa do Zero com o nó mutado
	origMutatedFull := make([]Hypervector, totalNodes)
	copy(origMutatedFull, orig)
	origMutatedFull[mutatedNode] = newVec

	hop1Full := hdc.PropagateHop1(graph, origMutatedFull)
	memRebuiltFull := hdc.PropagateHop2(graph, hop1Full)

	// 3. Caminho B: Atualização Incremental (afeta apenas o cone de dependência reversa)
	origMutatedInc := make([]Hypervector, totalNodes)
	copy(origMutatedInc, orig)

	contextualInc := make([]Hypervector, totalNodes)
	copy(contextualInc, memInit.Contextual)

	hasContextInc := make([]bool, totalNodes)
	copy(hasContextInc, memInit.HasContext)

	memInc := &ContextualMemory{
		Original:   origMutatedInc,
		Contextual: contextualInc,
		HasContext: hasContextInc,
	}

	t0 := time.Now()
	_, affected := hdc.UpdateNodeIncremental2Hop(graph, memInc, mutatedNode, newVec)
	durInc := time.Since(t0)

	t.Logf("Nó mutado: %d | Cone afetado: %d nós de %d total", mutatedNode, len(affected.AllAffected), totalNodes)
	t.Logf("Tempo de atualização incremental: %v", durInc)

	// 4. VERIFICAÇÃO RIGOROSA BYTE-A-BYTE:
	// Toda a memória contextual deve ser 100% IDÊNTICA entre a reconstrução total e a incremental!
	mismatches := 0
	for i := 0; i < totalNodes; i++ {
		if memInc.HasContext[i] != memRebuiltFull.HasContext[i] {
			t.Errorf("Discrepância em HasContext para o nó %d: Inc=%v, Full=%v", i, memInc.HasContext[i], memRebuiltFull.HasContext[i])
			mismatches++
		}
		if memInc.Contextual[i] != memRebuiltFull.Contextual[i] {
			diffBits := HammingDistance(memInc.Contextual[i], memRebuiltFull.Contextual[i])
			t.Errorf("Discrepância em Contextual para o nó %d: %d bits diferentes", i, diffBits)
			mismatches++
		}
	}

	if mismatches > 0 {
		t.Fatalf("FALHA DE CORRETUDE: %d nós divergiram entre a atualização incremental e a reconstrução total!", mismatches)
	}

	t.Logf("SUCESSO ABSOLUTO: Atualização incremental é 100%% idêntica byte-a-byte à reconstrução total!")
}

// Benchmark de Atualização Incremental vs Reconstrução Total em 1.000 nós
func BenchmarkIncrementalUpdate_1000Nodes(b *testing.B) {
	const totalNodes = 1000
	rng := rand.New(rand.NewSource(42))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 3; k++ {
			target := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: target, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)
	hop1 := hdc.PropagateHop1(graph, orig)
	mem := hdc.PropagateHop2(graph, hop1)

	newVec := GenerateRandomHypervector(rng)
	mutatedNode := uint16(500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hdc.UpdateNodeIncremental2Hop(graph, mem, mutatedNode, newVec)
	}
}


// Fixed regression: A -> B, B is a leaf. Full PropagateHop2 must fall back
// to the direct 1-hop embedding at A after B's original vector is changed.
func TestIncrementalTerminalLeafFallback(t *testing.T) {
	rng := rand.New(rand.NewSource(981))
	orig := []Hypervector{GenerateRandomHypervector(rng), GenerateRandomHypervector(rng)}
	g := BuildCSRFromAdj(2, [][]AdjEdge{
		{{Target: 1, Rel: RelCall}},
		nil,
	})
	sys := NewHDCSystem(1337)
	initial := sys.PropagateHop2(g, sys.PropagateHop1(g, orig))
	incOriginal := append([]Hypervector(nil), orig...)
	incCtx := append([]Hypervector(nil), initial.Contextual...)
	incHas := append([]bool(nil), initial.HasContext...)
	mem := &ContextualMemory{Original: incOriginal, Contextual: incCtx, HasContext: incHas}
	mutated := GenerateRandomHypervector(rng)
	rev := BuildReverseCSR(g)
	_, cone := sys.UpdateNodeIncremental2HopWithReverse(g, rev, mem, 1, mutated)
	if !cone.AllAffected[0] || !cone.AllAffected[1] {
		t.Fatalf("reverse dependency cone omitted A or B: %+v", cone)
	}
	fullOriginal := append([]Hypervector(nil), orig...)
	fullOriginal[1] = mutated
	want := sys.PropagateHop2(g, sys.PropagateHop1(g, fullOriginal))
	if !mem.HasContext[0] || mem.Contextual[0] != want.Contextual[0] {
		t.Fatal("terminal leaf fallback diverged from full PropagateHop2")
	}
	if mem.Contextual[0] == (Hypervector{}) {
		t.Fatal("fallback incorrectly returned the zero vector")
	}
	compareFullVsIncremental(t, mem, want)
}

func compareFullVsIncremental(t *testing.T, got, want *ContextualMemory) {
	t.Helper()
	if len(got.Original) != len(want.Original) ||
		len(got.Contextual) != len(want.Contextual) ||
		len(got.HasContext) != len(want.HasContext) {
		t.Fatalf("memory lengths differ")
	}
	for i := range want.Contextual {
		if got.Original[i] != want.Original[i] ||
			got.Contextual[i] != want.Contextual[i] ||
			got.HasContext[i] != want.HasContext[i] {
			t.Fatalf("node %d diverged: original=%v context_bits=%d flag=%v",
				i, got.Original[i] == want.Original[i],
				HammingDistance(got.Contextual[i], want.Contextual[i]),
				got.HasContext[i] == want.HasContext[i])
		}
	}
}

// Deterministic property sweep: 20 independently seeded topologies with
// isolated nodes, leaves, cycles, mixed relation types and high-degree hubs;
// five consecutive mutations on each graph, comparing every bit and flag.
func TestIncrementalMultiGraphFuzz(t *testing.T) {
	const seeds, nodes, mutations = 20, 72, 5
	for si := 0; si < seeds; si++ {
		rng := rand.New(rand.NewSource(int64(10000 + si)))
		orig := make([]Hypervector, nodes)
		adj := make([][]AdjEdge, nodes)
		for i := 0; i < nodes; i++ {
			orig[i] = GenerateRandomHypervector(rng)
		}
		for v := 0; v < nodes; v++ {
			degree := 1 + rng.Intn(5)
			if v%9 == 0 { degree = 0 } // genuine leaf
			if v%17 == 0 { degree = 0 } // isolated or incoming-only
			if v%13 == 1 { degree = 12 } // high-degree hub
			for j := 0; j < degree; j++ {
				target := uint16(rng.Intn(nodes))
				if int(target) == v { continue }
				rel := RelCall
				if j%7 == 1 { rel = RelMemberOf }
				if j%7 == 2 { rel = RelRevCall }
				adj[v] = append(adj[v], AdjEdge{Target: target, Rel: rel})
			}
		}
		// Add explicit bidirectional cycles without removing leaf fixtures.
		adj[1] = append(adj[1], AdjEdge{Target: 2, Rel: RelCall})
		adj[2] = append(adj[2], AdjEdge{Target: 1, Rel: RelRevCall})
		g := BuildCSRFromAdj(nodes, adj)
		reverse := BuildReverseCSR(g)
		system := NewHDCSystem(42 + int64(si))
		initial := system.PropagateHop2(g, system.PropagateHop1(g, orig))
		mem := &ContextualMemory{
			Original: append([]Hypervector(nil), orig...),
			Contextual: append([]Hypervector(nil), initial.Contextual...),
			HasContext: append([]bool(nil), initial.HasContext...),
		}
		oracleOrig := append([]Hypervector(nil), orig...)
		for step := 0; step < mutations; step++ {
			node := uint16(rng.Intn(nodes))
			vec := GenerateRandomHypervector(rng)
			oracleOrig[node] = vec
			_, cone := system.UpdateNodeIncremental2HopWithReverse(g, reverse, mem, node, vec)
			if !cone.AllAffected[node] {
				t.Fatalf("seed %d mutation %d: mutated node absent from invalidation cone", si, step)
			}
			want := system.PropagateHop2(g, system.PropagateHop1(g, oracleOrig))
			compareFullVsIncremental(t, mem, want)
		}
	}
}

// These two benchmarks intentionally reuse the SAME 1,000-node fixture and
// the SAME alternating sequence of vector mutations. The cached reverse
// index and initial memory are built OUTSIDE the incremental timed region.
var pairedMutationSink *ContextualMemory

func pairedMutationVectors() []Hypervector {
	rng := rand.New(rand.NewSource(8081))
	vecs := make([]Hypervector, 8)
	for i := range vecs { vecs[i] = GenerateRandomHypervector(rng) }
	return vecs
}

func BenchmarkPropagateHop2_1000Nodes_Paired(b *testing.B) {
	fixture := buildMultipathFixture(8080, 1000)
	sys := NewHDCSystem(42)
	mutations := pairedMutationVectors()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fixture.vectors[500] = mutations[i%len(mutations)]
		hop1 := sys.PropagateHop1(fixture.graph, fixture.vectors)
		pairedMutationSink = sys.PropagateHop2(fixture.graph, hop1)
	}
}

func BenchmarkIncrementalUpdate_1000Nodes_Paired(b *testing.B) {
	fixture := buildMultipathFixture(8080, 1000)
	sys := NewHDCSystem(42)
	rev := BuildReverseCSR(fixture.graph)
	mem := sys.PropagateHop2(fixture.graph, sys.PropagateHop1(fixture.graph, fixture.vectors))
	mutations := pairedMutationVectors()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pairedMutationSink, _ = sys.UpdateNodeIncremental2HopWithReverse(
			fixture.graph, rev, mem, 500, mutations[i%len(mutations)])
	}
}
