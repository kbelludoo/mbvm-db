package mbvmdb

import (
	"fmt"
	"math/bits"
	"sort"
)

// NeuralBlastRadiusResult consolida a resposta do pipeline completo Neuro-Simbólico.
type NeuralBlastRadiusResult struct {
	TargetSymbol        string
	TargetSymbolID      uint16
	SemanticCandidates  []SemanticCandidate // Candidatos filtrados pelo Estágio 1 (1-Bit Hamming 8K)
	ExactBlastRadius    []*SymbolRecord     // Símbolos afetados comprovados pelo Grafo CSR
	TotalNodesEvaluated int
	TotalAffectedNodes  int
}

// SemanticCandidate representa um símbolo identificado pela proximidade vetorial.
type SemanticCandidate struct {
	SymbolID        uint16
	Name            string
	HammingDistance int
	SimilarityScore float64 // Estimativa de similaridade de cosseno via hiperplanos
}

// QueryNeuralBlastRadius executa a consulta de ponta a ponta:
// 1. Localiza o símbolo alvo no banco determinístico.
// 2. Estágio 1 (Neural): Varre os vetores de 8.192 bits usando 128x (XOR + POPCNT) e extrai os Top-K semanticamente próximos.
// 3. Estágio 2 (Simbólico): Executa o Blast Radius sobre o grafo CSR (ExtractSlice com traversal em profundidade).
// 4. Retorna a decisão unificada pronta para a VM.
func (db *DB) QueryNeuralBlastRadius(targetSymbol string, neuralIdx *NeuralIndex, topKCandidates int, maxDepth int) (*NeuralBlastRadiusResult, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	targetRec, exists := db.indexByName[targetSymbol]
	if !exists {
		return nil, fmt.Errorf("símbolo alvo '%s' não encontrado no banco de dados", targetSymbol)
	}
	targetID := targetRec.ID

	result := &NeuralBlastRadiusResult{
		TargetSymbol:        targetSymbol,
		TargetSymbolID:      targetID,
		TotalNodesEvaluated: len(db.symbols),
	}

	// 1. Estágio 1: Varredura Neural 8K com Zero-Allocations
	if neuralIdx != nil && uint32(targetID) < neuralIdx.TotalSymbols {
		targetOffset := int(targetID) * NeuralWordsPerVec
		targetWords := neuralIdx.Words[targetOffset : targetOffset+NeuralWordsPerVec]

		candidates := make([]SemanticCandidate, 0, len(db.symbols))
		words := neuralIdx.Words

		for i := 0; i < len(db.symbols); i++ {
			sid := db.symbols[i].ID
			if sid == targetID || uint32(sid) >= neuralIdx.TotalSymbols {
				continue
			}

			candOffset := int(sid) * NeuralWordsPerVec
			var dist int
			for w := 0; w < NeuralWordsPerVec; w++ {
				dist += bits.OnesCount64(targetWords[w] ^ words[candOffset+w])
			}

			// CosSim ≈ 1.0 - (2.0 * dist / D)
			sim := 1.0 - (2.0 * float64(dist) / float64(NeuralVectorBits))

			candidates = append(candidates, SemanticCandidate{
				SymbolID:        sid,
				Name:            db.symbols[i].Name,
				HammingDistance: dist,
				SimilarityScore: sim,
			})
		}

		// Ordena os candidatos por menor distância de Hamming
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].HammingDistance < candidates[j].HammingDistance
		})

		if len(candidates) > topKCandidates {
			candidates = candidates[:topKCandidates]
		}
		result.SemanticCandidates = candidates
	}

	// 2. Estágio 2: Blast Radius Simbólico e Determinístico via Grafo CSR
	slice := db.extractSliceLocked(targetSymbol, maxDepth)
	if slice != nil {
		result.ExactBlastRadius = slice.Symbols
		result.TotalAffectedNodes = len(slice.Symbols)
	}

	return result, nil
}
