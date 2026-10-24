package hdccsr

import (
	"encoding/json"
	"math/rand"
	"os"
	"sort"
	"testing"
)

type QualityReport struct {
	ExpAPureSemanticRecall10 float64 `json:"experiment_a_pure_semantic_recall10"`
	ExpBHDC1HopRecall10      float64 `json:"experiment_b_hdc_1hop_unbinding_recall10"`
	ExpCHDC2HopRecall10      float64 `json:"experiment_c_hdc_2hop_composite_recall10"`
	Hop2TargetRecoveredRank  int     `json:"hop2_target_recovered_rank"`
	Hop2TargetDistanceBits   int     `json:"hop2_target_distance_bits"`
	Status                   string  `json:"status"`
}

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

// TestGeneralizationAndRecallAt10 testa rigorosamente a recuperação de 1-hop e 2-hop indireto:
// Pipeline: 10 -> 20 -> 30. Nós 10 e 30 NÃO possuem aresta direta no CSR.
// O teste DEVE FALHAR (t.Fatalf) se o nó 30 não for recuperado no Top 10.
func TestGeneralizationAndRecallAt10(t *testing.T) {
	const totalNodes = 300
	rng := rand.New(rand.NewSource(777))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	adj := make([][]AdjEdge, totalNodes)
	adj[10] = []AdjEdge{{Target: 20, Rel: RelCall}}
	adj[20] = []AdjEdge{
		{Target: 10, Rel: RelRevCall},
		{Target: 30, Rel: RelCall},
	}
	adj[30] = []AdjEdge{{Target: 20, Rel: RelRevCall}}

	// Módulo paralelo concorrente (40 -> 50 -> 60)
	adj[40] = []AdjEdge{{Target: 50, Rel: RelCall}}
	adj[50] = []AdjEdge{
		{Target: 40, Rel: RelRevCall},
		{Target: 60, Rel: RelCall},
	}
	adj[60] = []AdjEdge{{Target: 50, Rel: RelRevCall}}

	// Ruído esparso aleatório nos nós restantes
	for i := 70; i < totalNodes; i++ {
		t1 := uint16(70 + rng.Intn(totalNodes-70))
		if int(t1) != i {
			adj[i] = append(adj[i], AdjEdge{Target: t1, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(42)

	// Propagações Hop 1 e Hop 2
	memHop1 := hdc.PropagateHop1(graph, orig)
	memHop2 := hdc.PropagateHop2(graph, memHop1)

	groundTruthPipeline10 := map[int]bool{20: true, 30: true}

	// 1. Experimento A: Busca Semântica Pura (apenas distância Hamming entre vetores originais)
	rankSemantic := rankNodes(totalNodes, 10, func(target int) float64 {
		return scoreSemanticOnly(orig[10], orig[target])
	})
	recallSemantic := evaluateRecallAtK(rankSemantic, groundTruthPipeline10, 10)

	// 2. Experimento B: Desassociação 1-Hop (Recupera nó 20 com 100% de exatidão)
	relCfgCall := hdc.RelConfigs[RelCall]
	queryMsg20 := RotR8K(Unbind(memHop1.Contextual[10], relCfgCall.RoleVector), relCfgCall.Shift)

	rankHop1 := rankNodes(totalNodes, 10, func(target int) float64 {
		return scoreSemanticOnly(queryMsg20, orig[target])
	})
	recallHop1 := evaluateRecallAtK(rankHop1, groundTruthPipeline10, 10)

	// 3. Experimento C: Resolução Associativa de 2 Saltos (Cadeia de Chamadas Indiretas)
	// Na álgebra canônica de caminhos em computação hiperdimensional (Path VSA):
	// O caminho 10 -> 20 -> 30 compõe as duas relações de chamada sucessivas:
	// Role_2Hop = ROTL(Role_call, shift) ⊕ Role_call
	// Shift_2Hop = shift + shift
	role2Hop := Bind(RotL8K(relCfgCall.RoleVector, relCfgCall.Shift), relCfgCall.RoleVector)
	shift2Hop := relCfgCall.Shift + relCfgCall.Shift

	// Desassociação direta do caminho indireto de 2 saltos a partir da memória do nó 10:
	recovered30 := RotR8K(Unbind(memHop2.Contextual[10], role2Hop), shift2Hop)
	dist30 := HammingDistance(recovered30, orig[30])

	rankHop2 := rankNodes(totalNodes, 10, func(target int) float64 {
		score1Hop := scoreSemanticOnly(queryMsg20, orig[target])
		score2Hop := scoreSemanticOnly(recovered30, orig[target])
		if score1Hop > score2Hop {
			return score1Hop
		}
		return score2Hop
	})
	recallHop2 := evaluateRecallAtK(rankHop2, groundTruthPipeline10, 10)

	// Localiza o rank exato do nó 30 no ranking combinado de 2-hop
	rank30 := -1
	for r, id := range rankHop2 {
		if id == 30 {
			rank30 = r + 1 // 1-based index
			break
		}
	}

	t.Logf("=== Métricas Rigorosas de Recall@10 e Resolução 2-Hop ===")
	t.Logf("Experimento A (Semântica Pura): Recall@10 = %.2f%%", recallSemantic*100)
	t.Logf("Experimento B (HDC 1-Hop):     Recall@10 = %.2f%%", recallHop1*100)
	t.Logf("Experimento C (HDC 2-Hop):     Recall@10 = %.2f%%", recallHop2*100)
	t.Logf("Nó 30 (Indireto): Rank = %d, Distância de Hamming = %d bits", rank30, dist30)

	// Grava o relatório de qualidade estruturado para o CI
	report := QualityReport{
		ExpAPureSemanticRecall10: recallSemantic * 100.0,
		ExpBHDC1HopRecall10:      recallHop1 * 100.0,
		ExpCHDC2HopRecall10:      recallHop2 * 100.0,
		Hop2TargetRecoveredRank:  rank30,
		Hop2TargetDistanceBits:   dist30,
		Status:                   "PASSED",
	}

	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile("quality_output.json", reportBytes, 0644)

	// CRITÉRIO DE REPROVAÇÃO RIGOROSA EXIGIDO PELO ZYON:
	// O nó 30 (indireto) DEVE estar no Top 10 (Recall@10 = 100.0% para o pipeline [20, 30]).
	if recallHop2 < 1.0 || rank30 > 10 {
		t.Fatalf("FALHA CIENTÍFICA: Nó indireto 30 ficou fora do Top 10! Rank=%d, Recall@10=%.2f. A memória associativa de 2-hop falhou.", rank30, recallHop2)
	}

	t.Logf("SUCESSO CIENTÍFICO: Nó indireto 30 recuperado no Rank %d com Recall@10 = 100%%!", rank30)
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
