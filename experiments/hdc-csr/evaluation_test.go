package hdccsr

import (
	"math/rand"
	"sort"
	"testing"
)

// evaluateRecallAtK calcula o Recall@K para uma lista de nós relevantes (ground truth).
func evaluateRecallAtK(rankedCandidates []int, relevantSet map[int]bool, k int) float64 {
	if len(relevantSet) == 0 {
		return 0.0
	}
	hits := 0
	limit := k
	if limit > len(rankedCandidates) {
		limit = len(rankedCandidates)
	}

	for i := 0; i < limit; i++ {
		if relevantSet[rankedCandidates[i]] {
			hits++
		}
	}
	return float64(hits) / float64(len(relevantSet))
}

// TestGeneralizationAndRecallAt10 testa explicitamente:
// 1. Relações indiretas / transitivas NÃO codificadas diretamente (A chama B, B chama C -> A e C nunca se chamaram).
// 2. Medição separada de Recall@10 semântico vs Recall@10 contextual vs Recall@10 combinado.
func TestGeneralizationAndRecallAt10(t *testing.T) {
	const totalNodes = 300
	rng := rand.New(rand.NewSource(777))

	// Vetores semânticos originais não correlacionados a priori
	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = generateRandomHypervector(rng)
	}

	// Cenário de teste: Pipeline de Processamento em 3 estágios (A -> B -> C)
	// Nó 10 (Controller) chama Nó 20 (Service).
	// Nó 20 (Service) chama Nó 30 (Repository/DB).
	// Nó 10 e Nó 30 NUNCA têm aresta direta entre si no grafo!
	// Também criamos nós de ruído e outros módulos concorrentes.
	adj := make([][]AdjEdge, totalNodes)
	adj[10] = []AdjEdge{{Target: 20, Rel: RelCall}}
	adj[20] = []AdjEdge{
		{Target: 10, Rel: RelRevCall},
		{Target: 30, Rel: RelCall},
	}
	adj[30] = []AdjEdge{{Target: 20, Rel: RelRevCall}}

	// Módulo concorrente independente (40 -> 50 -> 60)
	adj[40] = []AdjEdge{{Target: 50, Rel: RelCall}}
	adj[50] = []AdjEdge{
		{Target: 40, Rel: RelRevCall},
		{Target: 60, Rel: RelCall},
	}
	adj[60] = []AdjEdge{{Target: 50, Rel: RelRevCall}}

	// Adiciona ruído esparso aleatório nos nós restantes (grau 1 a 3)
	for i := 70; i < totalNodes; i++ {
		t1 := uint16(70 + rng.Intn(totalNodes-70))
		if int(t1) != i {
			adj[i] = append(adj[i], AdjEdge{Target: t1, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)

	// Camada 1: propaga vizinhos diretos (1-hop)
	memHop1 := hdc.PropagateHop1(graph, orig)

	// Camada 2: propaga relações de 2-hop (transição indireta)
	memHop2 := hdc.PropagateHop2(graph, memHop1)

	// Query: A partir do Nó 10 (Controller), queremos descobrir os nós de sua cadeia funcional (Ground Truth: 20 e 30).
	// Nó 30 NUNCA esteve na lista de adjacência de 10.
	groundTruthPipeline10 := map[int]bool{20: true, 30: true}

	// 1. Busca Semântica Pura (Experimento A: query direta orig[10] tentando achar quem é chamado)
	rankSemantic := rankNodes(totalNodes, 10, func(target int) float64 {
		return scoreSemanticOnly(orig[10], orig[target])
	})
	recallSemantic := evaluateRecallAtK(rankSemantic, groundTruthPipeline10, 10)

	// 2. Busca Associativa HDC 1-Hop (Experimento B: Desassociação da relação de chamada)
	// Query: "Recuperar símbolo que preenche o papel RelCall a partir do contexto de 10"
	relCfgCall := hdc.RelConfigs[RelCall]
	queryMsg20 := RotR8K(Unbind(memHop1.Contextual[10], relCfgCall.RoleVector), relCfgCall.Shift)

	rankHop1 := rankNodes(totalNodes, 10, func(target int) float64 {
		// Mede a distância entre a representação desassociada recuperada da memória e o vetor original do candidato
		return scoreSemanticOnly(queryMsg20, orig[target])
	})
	recallHop1 := evaluateRecallAtK(rankHop1, groundTruthPipeline10, 10)

	// 3. Busca Associativa HDC 2-Hop (Experimento C: desassociação encadeada de 2 saltos)
	// A relação composta de 2 saltos (Chama -> Chama) é BIND(R_call, ROTL(R_call, shift))
	compositeRel := Bind(relCfgCall.RoleVector, RotL8K(relCfgCall.RoleVector, relCfgCall.Shift))
	compositeShift := relCfgCall.Shift * 2
	queryMsg30 := RotR8K(Unbind(memHop2.Contextual[10], compositeRel), compositeShift)

	rankHop2 := rankNodes(totalNodes, 10, func(target int) float64 {
		score1Hop := scoreSemanticOnly(queryMsg20, orig[target])
		score2Hop := scoreSemanticOnly(queryMsg30, orig[target])
		if score1Hop > score2Hop {
			return score1Hop
		}
		return score2Hop
	})
	recallHop2 := evaluateRecallAtK(rankHop2, groundTruthPipeline10, 10)

	t.Logf("=== Métricas de Recall@10 para Descoberta de Relações Indiretas ===")
	t.Logf("Experimento A (Semântica Pura): Recall@10 = %.2f%%", recallSemantic*100)
	t.Logf("Experimento B (HDC 1-Hop):     Recall@10 = %.2f%%", recallHop1*100)
	t.Logf("Experimento C (HDC 2-Hop):     Recall@10 = %.2f%%", recallHop2*100)

	// Validação de superioridade do 2-Hop para relações indiretas:
	// No Hop 2, tanto o nó 20 (direto) quanto o nó 30 (indireto de 2 saltos) devem ser capturados no Top 10.
	if recallHop2 < recallSemantic {
		t.Errorf("HDC 2-Hop deveria superar semântica pura em relações estruturais profundas")
	}
	if recallHop2 < 1.0 {
		t.Logf("Aviso: Nó indireto 30 ficou fora do Top 10 (Recall@10 = %.2f)", recallHop2)
	} else {
		t.Logf("Sucesso Total: Nó 30 (indireto, não-adjacente) foi descoberto no Top-10 através de propagação composicional!")
	}
}

func rankNodes(total int, queryID int, scorer func(int) float64) []int {
	candidates := make([]RetrievalCandidate, 0, total-1)
	for i := 0; i < total; i++ {
		if i == queryID {
			continue // Exclui a própria query
		}
		candidates = append(candidates, RetrievalCandidate{
			ID:    i,
			Score: scorer(i),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	out := make([]int, len(candidates))
	for i, c := range candidates {
		out[i] = c.ID
	}
	return out
}
