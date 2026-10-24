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
