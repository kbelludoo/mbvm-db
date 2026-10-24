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

// TestNeuralQualityCascadeRecall mede o Recall real na arquitetura em cascata:
// O Estágio 1 (1-Bit Hamming 8K) filtra uma janela maior (ex: Top-50),
// e o Estágio 2 faz o refinamento dos Top-10.
func TestNeuralQualityCascadeRecall(t *testing.T) {
	rand.Seed(42)

	const numNodes = 1000
	const denseDims = 384
	const numBits = 8192
	const numWords = 128
	const topK = 10
	const cascadeCandidateWindow = 50 // Janela de candidatos do Estágio 1
	const numQueries = 50

	// 1. Gera embeddings densos com clusters semânticos
	denseVectors := make([][]float32, numNodes)
	numClusters := 10
	clusterCenters := make([][]float32, numClusters)
	for c := 0; c < numClusters; c++ {
		clusterCenters[c] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			clusterCenters[c][d] = rand.Float32()*2 - 1
		}
	}

	for i := 0; i < numNodes; i++ {
		center := clusterCenters[i%numClusters]
		denseVectors[i] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			denseVectors[i][d] = center[d] + (rand.Float32()*0.4 - 0.2)
		}
	}

	// 2. Matriz de projeção
	projMatrix := make([][]float32, numBits)
	for b := 0; b < numBits; b++ {
		projMatrix[b] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			projMatrix[b][d] = rand.Float32()*2 - 1
		}
	}

	// 3. Projeta para 8.192 bits
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

	// 4. Executa queries e avalia o Recall no funil em cascata
	var totalRecallCascade float64
	var totalRecallTop10Pure float64

	for q := 0; q < numQueries; q++ {
		queryID := uint32(rand.Intn(numNodes))

		// Ground Truth: Top-10 por Cosseno Float32
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

		// Hamming: ordena todos por distância de Hamming
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

		// A. Pure Recall@10
		matchesPure := 0
		for i := 0; i < topK; i++ {
			if topKGroundTruth[hammingScores[i].ID] {
				matchesPure++
			}
		}
		totalRecallTop10Pure += float64(matchesPure) / float64(topK)

		// B. Cascade Recall: Quantos dos Top-10 reais foram capturados na janela de 50 candidatos do 1-Bit?
		matchesCascade := 0
		for i := 0; i < cascadeCandidateWindow && i < len(hammingScores); i++ {
			if topKGroundTruth[hammingScores[i].ID] {
				matchesCascade++
			}
		}
		totalRecallCascade += float64(matchesCascade) / float64(topK)
	}

	avgRecallPure := (totalRecallTop10Pure / float64(numQueries)) * 100.0
	avgRecallCascade := (totalRecallCascade / float64(numQueries)) * 100.0

	t.Logf("=================================================================")
	t.Logf("🔬 PROVA EXPERIMENTAL REAL: 1-BIT HAMMING COMO FILTRO DE CASCA")
	t.Logf("   Pure Top-10 Recall (Hamming direto): %.2f%%", avgRecallPure)
	t.Logf("   Cascade Top-50 -> Top-10 Recall:     %.2f%%", avgRecallCascade)
	t.Logf("=================================================================")

	if avgRecallCascade < 85.0 {
		t.Fatalf("Cascade Recall insuficiente: %.2f%%", avgRecallCascade)
	}
}
