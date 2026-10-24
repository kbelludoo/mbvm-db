package mbvmdb

import (
	"fmt"
	"math/bits"
	"math/rand"
	"sort"
	"testing"
)

// TestRobustnessBitMaskingExperiment avalia a hipótese "Sans Forgetica" (Dificuldade Desejável / HDC Robustness):
// Mede como o Recall@10 do hipervetor de 8.192 bits degrada quando submetido a diferentes níveis
// de corrupção/apagamento de bits (0%, 5%, 10%, 15%, 20%, 25%, 30%, 40%).
func TestRobustnessBitMaskingExperiment(t *testing.T) {
	// RNG determinístico isolado
	rng := rand.New(rand.NewSource(42))

	const numNodes = 1000
	const denseDims = 384
	const numBits = 8192
	const numWords = 128
	const topK = 10
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

	// 2. Projeção LSH para 8.192 bits
	projMatrix := make([][]float32, numBits)
	for b := 0; b < numBits; b++ {
		projMatrix[b] = make([]float32, denseDims)
		for d := 0; d < denseDims; d++ {
			projMatrix[b][d] = rng.Float32()*2 - 1
		}
	}

	cleanIdx := NewNeuralIndex(numNodes)
	for i := 0; i < numNodes; i++ {
		bitWords := make([]uint64, numWords)
		for b := 0; b < numBits; b++ {
			var dot float32
			for d := 0; d < denseDims; d++ {
				dot += denseVectors[i][d] * projMatrix[b][d]
			}
			if dot > 0 {
				bitWords[b/64] |= (1 << (b % 64))
			}
		}
		_ = cleanIdx.SetVector(uint32(i), bitWords)
	}

	// Níveis de corrupção/apagamento de bits a testar (Bit Flip / Noise)
	corruptionRates := []float64{0.00, 0.05, 0.10, 0.15, 0.20, 0.25, 0.30, 0.40}

	t.Logf("==========================================================================================")
	t.Logf("🔬 EXPERIMENTO ZYON: ROBUSTEZ DA MEMÓRIA HDC 8K A CORRUPÇÃO/APAGAMENTO DE BITS")
	t.Logf("   Dimensão: 8.192 bits | Total de Nós: %d | Queries: %d | Top-K: %d", numNodes, numQueries, topK)
	t.Logf("==========================================================================================")
	t.Logf("%-18s | %-15s | %-18s | %-15s", "Corrupção (% bits)", "Bits Invertidos", "Recall@10 Cascade", "Retenção (%)")
	t.Logf("------------------------------------------------------------------------------------------")

	var baselineRecall float64

	for rateIdx, rate := range corruptionRates {
		bitsToFlip := int(float64(numBits) * rate)

		var totalCascadeRecall float64

		for q := 0; q < numQueries; q++ {
			queryID := uint32(rng.Intn(numNodes))

			// Ground Truth Limpo (Float32 Cosine)
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

			// Gera Query Corrompida com Bits Invertidos/Ocultados
			queryVecClean := cleanIdx.GetVector(queryID)
			queryVecCorrupted := make([]uint64, numWords)
			copy(queryVecCorrupted, queryVecClean)

			// Inverte aleatoriamente 'bitsToFlip' posições
			if bitsToFlip > 0 {
				perm := rng.Perm(numBits)
				for f := 0; f < bitsToFlip; f++ {
					bitPos := perm[f]
					wordIdx := bitPos / 64
					bitIdx := bitPos % 64
					queryVecCorrupted[wordIdx] ^= (1 << bitIdx)
				}
			}

			// Varre todos os nós usando o vetor corrompido
			hammingScores := make([]Pair, 0, numNodes-1)
			for i := 0; i < numNodes; i++ {
				if uint32(i) == queryID {
					continue
				}
				targetVec := cleanIdx.GetVector(uint32(i))
				var dist int
				for w := 0; w < numWords; w++ {
					dist += bits.OnesCount64(queryVecCorrupted[w] ^ targetVec[w])
				}
				hammingScores = append(hammingScores, Pair{ID: uint32(i), Score: float64(dist)})
			}
			sort.Slice(hammingScores, func(i, j int) bool {
				return hammingScores[i].Score < hammingScores[j].Score
			})

			// Mede Recall da janela de 64 candidatos
			matches := 0
			for i := 0; i < 64 && i < len(hammingScores); i++ {
				if topKGroundTruth[hammingScores[i].ID] {
					matches++
				}
			}
			totalCascadeRecall += float64(matches) / float64(topK)
		}

		avgRecall := (totalCascadeRecall / float64(numQueries)) * 100.0
		if rateIdx == 0 {
			baselineRecall = avgRecall
		}
		retention := (avgRecall / baselineRecall) * 100.0

		t.Logf("%-18s | %-15d | %-17.2f%% | %-14.2f%%",
			fmt.Sprintf("%.1f%%", rate*100),
			bitsToFlip,
			avgRecall,
			retention,
		)
	}

	t.Logf("==========================================================================================")
}
