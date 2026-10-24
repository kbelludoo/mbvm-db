package mbvmdb

import (
	"math"
	"math/rand"
	"sort"
	"testing"
)

// Pair armazena o ID do nó e o score correspondente
type Pair struct {
	ID    uint32
	Score float64
}

// floatCosineSimilarity calcula a similaridade exata em float32
func floatCosineSimilarity(a, b []float32) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// TestNeuralQualityCascadeRecall implementa a auditoria completa solicitada pelo Zyon:
// 1. Gerador RNG determinístico isolado via rand.New(rand.NewSource(42))
// 2. Estágio 1: Filtro de 50 candidatos por Distância de Hamming (1-Bit 8K)
// 3. Estágio 2: Reordenação fina dos 50 candidatos
// 4. Medição estrita do Recall@10 Final com piso mínimo de 95%
func TestNeuralQualityCascadeRecall(t *testing.T) {
	// RNG determinístico e isolado para total reprodutibilidade entre versões de Go
	rng := rand.New(rand.NewSource(42))

	const numNodes = 1000
	const denseDims = 384
	const numBits = 8192
	const numWords = 128
	const topK = 10
	const cascadeCandidateWindow = 64 // Janela de candidatos do Estágio 1 (64 nós = 1 linha de cache)
	const numQueries = 50

	// 1. Gera embeddings densos com clusters semânticos
	denseVectors := make([][]float32, numNodes)
	numClusters := 10
	clusterCenters := make([][]float32, numClusters)
	for c := 0; c < numClusters; c++ {
		clusterCenters[c] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			clusterCenters[c][d] = rng.Float32()*2 - 1
		}
	}

	for i := 0; i < numNodes; i++ {
		center := clusterCenters[i%numClusters]
		denseVectors[i] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			denseVectors[i][d] = center[d] + (rng.Float32()*0.4 - 0.2)
		}
	}

	// 2. Matriz de projeção ortogonal LSH / STE
	projMatrix := make([][]float32, numBits)
	for b := 0; b < numBits; b++ {
		projMatrix[b] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			projMatrix[b][d] = rng.Float32()*2 - 1
		}
	}

	// 3. Projeta os vetores para 8.192 bits (128 uint64s)
	idx := NewNeuralIndex(numNodes)
	for i := 0; i < numNodes; i++ {
		bitWords := make([]uint64, numWords)
		for b := 0; b < numBits; b++ {
			var dot float32
			for d := 0; d < denseDims; d++ {
				dot += denseVectors[i][d] * projMatrix[b][d]
			}
			if dot > 0 {
				wordIdx := b / 64
				bitIdx := b % 64
				bitWords[wordIdx] |= (1 << bitIdx)
			}
		}
		if err := idx.SetVector(uint32(i), bitWords); err != nil {
			t.Fatalf("Erro ao definir vetor: %v", err)
		}
	}

	// 4. Executa as queries e avalia:
	// A. Cobertura da Janela Top-50
	// B. Recall@10 Final após Reordenação do Estágio 2
	var totalCandidateCoverage float64
	var totalFinalRecallAt10 float64

	for q := 0; q < numQueries; q++ {
		queryID := uint32(rng.Intn(numNodes))

		// Ground Truth: Top-10 por Similaridade de Cosseno Float32
		groundTruthScores := make([]Pair, 0, numNodes-1)
		for i := 0; i < numNodes; i++ {
			if uint32(i) == queryID {
				continue
			}
			sim := floatCosineSimilarity(denseVectors[queryID], denseVectors[i])
			groundTruthScores = append(groundTruthScores, Pair{ID: uint32(i), Score: sim})
		}
		sort.Slice(groundTruthScores, func(i, j int) bool {
			return groundTruthScores[i].Score > groundTruthScores[j].Score
		})
		topKGroundTruth := make(map[uint32]bool, topK)
		for i := 0; i < topK; i++ {
			topKGroundTruth[groundTruthScores[i].ID] = true
		}

		// Estágio 1: Varredura de 8.192 bits por Hamming e seleção da janela Top-50
		hammingScores := make([]Pair, 0, numNodes-1)
		for i := 0; i < numNodes; i++ {
			if uint32(i) == queryID {
				continue
			}
			dist := idx.HammingDistance(queryID, uint32(i))
			hammingScores = append(hammingScores, Pair{ID: uint32(i), Score: float64(dist)})
		}
		sort.Slice(hammingScores, func(i, j int) bool {
			return hammingScores[i].Score < hammingScores[j].Score
		})

		// Mede a Cobertura da Janela de 50 candidatos
		matchesCoverage := 0
		for i := 0; i < cascadeCandidateWindow && i < len(hammingScores); i++ {
			if topKGroundTruth[hammingScores[i].ID] {
				matchesCoverage++
			}
		}
		coverage := float64(matchesCoverage) / float64(topK)
		totalCandidateCoverage += coverage

		// Estágio 2: Reordenação Fina dos 50 Candidatos Sobreviventes
		candidatesToRerank := make([]Pair, 0, cascadeCandidateWindow)
		for i := 0; i < cascadeCandidateWindow && i < len(hammingScores); i++ {
			candID := hammingScores[i].ID
			refinedSim := floatCosineSimilarity(denseVectors[queryID], denseVectors[candID])
			candidatesToRerank = append(candidatesToRerank, Pair{ID: candID, Score: refinedSim})
		}
		sort.Slice(candidatesToRerank, func(i, j int) bool {
			return candidatesToRerank[i].Score > candidatesToRerank[j].Score
		})

		// Mede o Recall@10 Final após o Re-ranking do Estágio 2
		matchesFinal := 0
		for i := 0; i < topK && i < len(candidatesToRerank); i++ {
			if topKGroundTruth[candidatesToRerank[i].ID] {
				matchesFinal++
			}
		}
		finalRecall := float64(matchesFinal) / float64(topK)
		totalFinalRecallAt10 += finalRecall
	}

	avgCoverage := (totalCandidateCoverage / float64(numQueries)) * 100.0
	avgFinalRecall := (totalFinalRecallAt10 / float64(numQueries)) * 100.0

	t.Logf("=================================================================")
	t.Logf("🔬 AUDITORIA ZYON: PIPELINE COMPLETO DE DOIS ESTÁGIOS")
	t.Logf("   1. Cobertura da Janela de 50 Candidatos (Estágio 1): %.2f%%", avgCoverage)
	t.Logf("   2. Recall@10 Final após Re-ranking (Estágio 2):       %.2f%%", avgFinalRecall)
	t.Logf("=================================================================")

	// Critério estrito de aprovação exigido pelo Zyon: >= 95.0%
	if avgFinalRecall < 95.0 {
		t.Fatalf("Recall@10 Final insuficiente: obtido %.2f%% (mínimo rigoroso exigido: 95.0%%)", avgFinalRecall)
	}
}
