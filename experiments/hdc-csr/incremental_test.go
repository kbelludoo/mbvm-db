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
