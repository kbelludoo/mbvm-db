package hdccsr

import (
	"math/rand"
	"testing"
)

// TestIncrementalTerminalLeafFallback testa especificamente o contra-exemplo do Zyon:
// Nó A chama B, e B é folha terminal (não chama ninguém).
// Ao mutar B, A deve receber o fallback de 1-hop idêntico à reconstrução completa.
func TestIncrementalTerminalLeafFallback(t *testing.T) {
	const totalNodes = 5
	rng := rand.New(rand.NewSource(101))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	// Topologia: 0 -> 1 (onde 1 é nó folha sem saídas), 2 -> 3 -> 4
	adj := make([][]AdjEdge, totalNodes)
	adj[0] = []AdjEdge{{Target: 1, Rel: RelCall}}
	adj[2] = []AdjEdge{{Target: 3, Rel: RelCall}}
	adj[3] = []AdjEdge{{Target: 4, Rel: RelCall}}

	graph := BuildCSRFromAdj(totalNodes, adj)
	revGraph := BuildRevCSR(graph)
	hdc := NewHDCSystem(123)

	hop1Init := hdc.PropagateHop1(graph, orig)
	memInit := hdc.PropagateHop2(graph, hop1Init)

	// Muta o nó folha 1
	mutatedNode := uint16(1)
	newVec := GenerateRandomHypervector(rand.New(rand.NewSource(202)))

	// Reconstrução total
	origFull := make([]Hypervector, totalNodes)
	copy(origFull, orig)
	origFull[mutatedNode] = newVec
	memFull := hdc.PropagateHop2(graph, hdc.PropagateHop1(graph, origFull))

	// Atualização incremental
	origInc := make([]Hypervector, totalNodes)
	copy(origInc, orig)
	ctxInc := make([]Hypervector, totalNodes)
	copy(ctxInc, memInit.Contextual)
	hasCtxInc := make([]bool, totalNodes)
	copy(hasCtxInc, memInit.HasContext)

	memInc := &ContextualMemory{
		Original:   origInc,
		Contextual: ctxInc,
		HasContext: hasCtxInc,
	}

	hdc.UpdateNodeIncremental2Hop(graph, revGraph, memInc, mutatedNode, newVec)

	// Validação de A (nó 0):
	if memInc.HasContext[0] != memFull.HasContext[0] {
		t.Fatalf("HasContext do nó 0 divergiu: Inc=%v, Full=%v", memInc.HasContext[0], memFull.HasContext[0])
	}
	if memInc.Contextual[0] != memFull.Contextual[0] {
		diff := HammingDistance(memInc.Contextual[0], memFull.Contextual[0])
		t.Fatalf("FALHA: Nó 0 com vizinho terminal divergiu por %d bits!", diff)
	}

	t.Logf("PASS: Fallback de 1-hop em nós com vizinhos terminais validado com 0 bits de discrepância.")
}

// TestIncrementalMultiGraphFuzz testa a equivalência estrita em 20 grafos aleatórios com ciclos e nós desconexos.
func TestIncrementalMultiGraphFuzz(t *testing.T) {
	for seed := int64(1); seed <= 20; seed++ {
		const totalNodes = 60
		rng := rand.New(rand.NewSource(seed))

		orig := make([]Hypervector, totalNodes)
		for i := 0; i < totalNodes; i++ {
			orig[i] = GenerateRandomHypervector(rng)
		}

		// Gera grafo com ciclos, folhas e nós de alto grau
		adj := make([][]AdjEdge, totalNodes)
		for i := 0; i < totalNodes; i++ {
			deg := rng.Intn(5) // Grau de 0 a 4
			for d := 0; d < deg; d++ {
				tgt := uint16(rng.Intn(totalNodes))
				if int(tgt) != i {
					adj[i] = append(adj[i], AdjEdge{Target: tgt, Rel: RelCall})
				}
			}
		}

		graph := BuildCSRFromAdj(totalNodes, adj)
		revGraph := BuildRevCSR(graph)
		hdc := NewHDCSystem(seed * 7)

		hop1 := hdc.PropagateHop1(graph, orig)
		memInc := hdc.PropagateHop2(graph, hop1)

		// Aplica 3 mutações consecutivas
		for m := 0; m < 3; m++ {
			mutNode := uint16(rng.Intn(totalNodes))
			newVec := GenerateRandomHypervector(rng)

			// Atualização incremental
			hdc.UpdateNodeIncremental2Hop(graph, revGraph, memInc, mutNode, newVec)

			// Reconstrução total independente
			origFull := make([]Hypervector, totalNodes)
			copy(origFull, memInc.Original)
			memFull := hdc.PropagateHop2(graph, hdc.PropagateHop1(graph, origFull))

			// Compara byte-a-byte todos os nós
			for node := 0; node < totalNodes; node++ {
				if memInc.HasContext[node] != memFull.HasContext[node] {
					t.Fatalf("[Seed %d, Mut %d] HasContext divergiu no nó %d", seed, m, node)
				}
				if memInc.Contextual[node] != memFull.Contextual[node] {
					diff := HammingDistance(memInc.Contextual[node], memFull.Contextual[node])
					t.Fatalf("[Seed %d, Mut %d] Contextual divergiu no nó %d: %d bits diferentes", seed, m, node, diff)
				}
			}
		}
	}
	t.Logf("PASS: Fuzz testing de atualização incremental em 20 grafos aleatórios com 3 mutações consecutivas: 100%% idêntico!")
}

// Benchmarks rigorosamente pareados sobre o mesmo grafo de 1.000 nós
func BenchmarkPropagateHop2_1000Nodes_SameGraph(b *testing.B) {
	const totalNodes = 1000
	rng := rand.New(rand.NewSource(42))
	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 3; k++ {
			tgt := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: tgt, Rel: RelCall})
		}
	}
	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		hop1 := hdc.PropagateHop1(graph, orig)
		_ = hdc.PropagateHop2(graph, hop1)
	}
}

func BenchmarkIncrementalUpdate_1000Nodes_SameGraph(b *testing.B) {
	const totalNodes = 1000
	rng := rand.New(rand.NewSource(42))
	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 3; k++ {
			tgt := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: tgt, Rel: RelCall})
		}
	}
	graph := BuildCSRFromAdj(totalNodes, adj)
	revGraph := BuildRevCSR(graph)
	hdc := NewHDCSystem(42)
	hop1 := hdc.PropagateHop1(graph, orig)
	mem := hdc.PropagateHop2(graph, hop1)

	newVec := GenerateRandomHypervector(rng)
	mutNode := uint16(500)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = hdc.UpdateNodeIncremental2Hop(graph, revGraph, mem, mutNode, newVec)
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
