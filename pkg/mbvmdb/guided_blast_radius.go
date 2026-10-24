package mbvmdb

import (
	"fmt"
	"math/bits"
	"sort"
)

// ImpactSeverity classifica o nível de risco e impacto com terminologia precisa.
type ImpactSeverity string

const (
	SeverityDirectDependency    ImpactSeverity = "DIRECT_DEPENDENCY"    // Chamador direto (CSR Reverso Nível 1)
	SeverityIndirectDependency  ImpactSeverity = "INDIRECT_DEPENDENCY"  // Chamador transitivo (CSR Reverso Nível > 1)
	SeveritySemanticAssociation ImpactSeverity = "SEMANTIC_ASSOCIATION" // Afinidade semântica sem aresta estrutural
)

// QueryMode define o modo de execução da consulta Neuro-Simbólica.
type QueryMode string

const (
	// ModeExact preserva 100% das dependências estruturais do CSR (Zero Falsos Negativos).
	// A rede neural é usada para priorizar, calcular o risco combinado e descobrir associações semânticas.
	ModeExact QueryMode = "EXACT"

	// ModeFastApproximate permite poda de ramos com alta divergência semântica para exploração ultrarrápida.
	ModeFastApproximate QueryMode = "FAST_APPROXIMATE"
)

// GuidedImpactNode representa um nó avaliado conjuntamente pelo Grafo e pela Rede Neural.
type GuidedImpactNode struct {
	Symbol          *SymbolRecord  `json:"symbol"`
	Severity        ImpactSeverity `json:"severity"`
	GraphDepth      int            `json:"graph_depth"`       // 0 = alvo, 1 = direto, >1 = indireto, -1 = sem aresta direta
	HammingDistance int            `json:"hamming_distance"`  // Distância calculada em 8.192 bits
	SemanticScore   float64        `json:"semantic_score"`    // [-1.0, 1.0]
	CombinedRisk    float64        `json:"combined_risk"`     // Score ponderado neuro-simbólico
	Reason          string         `json:"reason"`
}

// GuidedBlastRadiusResult consolida a decisão com transparência total de modo e contadores.
type GuidedBlastRadiusResult struct {
	TargetSymbol              string              `json:"target_symbol"`
	TargetID                  uint16              `json:"target_id"`
	Mode                      QueryMode           `json:"mode"`
	TotalEvaluated            int                 `json:"total_evaluated"`
	ImpactedNodes             []*GuidedImpactNode `json:"impacted_nodes"`
	DeterministicCallersCount int                 `json:"deterministic_callers_count"`
	SemanticAssociationCount  int                 `json:"semantic_association_count"`
	PrunedBranchesCount       int                 `json:"pruned_branches_count"`
}

// QueryGuidedNeuralBlastRadius implementa os dois modos fundamentais solicitados pelo Zyon:
// - ModeExact: NUNCA descarta dependências comprovadas do CSR. A IA atua na ordenação por risco e na descoberta semântica.
// - ModeFastApproximate: Permite poda neural atenuando ramos periféricos ruidosos.
func (db *DB) QueryGuidedNeuralBlastRadius(
	targetSymbol string,
	neuralIdx *NeuralIndex,
	mode QueryMode,
	maxDepth int,
	semanticThreshold float64,
	pruneHammingThreshold int,
) (*GuidedBlastRadiusResult, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	targetRec, exists := db.indexByName[targetSymbol]
	if !exists {
		return nil, fmt.Errorf("símbolo alvo '%s' não encontrado no banco de dados", targetSymbol)
	}
	targetID := targetRec.ID

	if mode == "" {
		mode = ModeExact
	}

	result := &GuidedBlastRadiusResult{
		TargetSymbol:   targetSymbol,
		TargetID:       targetID,
		Mode:           mode,
		TotalEvaluated: len(db.symbols),
	}

	if pruneHammingThreshold <= 0 {
		pruneHammingThreshold = 3800
	}

	// 1. Pré-calcula vetor semântico do alvo
	var targetWords []uint64
	hasNeural := neuralIdx != nil && uint32(targetID) < neuralIdx.TotalSymbols
	if hasNeural {
		targetOffset := int(targetID) * NeuralWordsPerVec
		targetWords = neuralIdx.Words[targetOffset : targetOffset+NeuralWordsPerVec]
	}

	getHamming := func(symID uint16) (int, float64, bool) {
		if !hasNeural || uint32(symID) >= neuralIdx.TotalSymbols {
			return 4096, 0.0, false // Vetor ausente ou neutro
		}
		candOffset := int(symID) * NeuralWordsPerVec
		candWords := neuralIdx.Words[candOffset : candOffset+NeuralWordsPerVec]
		var dist int
		for w := 0; w < NeuralWordsPerVec; w++ {
			dist += bits.OnesCount64(targetWords[w] ^ candWords[w])
		}
		sim := 1.0 - (2.0 * float64(dist) / float64(NeuralVectorBits))
		return dist, sim, true
	}

	// 2. Travessia Simbólica de Impacto Reverso (CSR Reverso)
	visited := NewBitset(len(db.symbols) + 64)
	visited.Set(targetID)

	type queueItem struct {
		id    uint16
		depth int
	}
	queue := []queueItem{{id: targetID, depth: 0}}
	impactMap := make(map[uint16]*GuidedImpactNode)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		if curr.depth >= maxDepth {
			continue
		}

		if int(curr.id) < len(db.csrRevOffsets)-1 {
			start := db.csrRevOffsets[curr.id]
			end := db.csrRevOffsets[curr.id+1]
			for e := start; e < end; e++ {
				callerID := db.csrRevEdges[e]
				if !visited.Has(callerID) {
					dist, sim, hasVec := getHamming(callerID)

					// PODA NEURAL: Executada APENAS no Modo FastApproximate
					// No Modo Exact, NUNCA podamos nós do CSR para garantir ZERO falsos negativos!
					if mode == ModeFastApproximate && hasVec && dist > pruneHammingThreshold {
						result.PrunedBranchesCount++
						continue
					}

					visited.Set(callerID)

					graphWeight := 1.0 / float64(curr.depth+1)
					combinedRisk := graphWeight * (1.0 + 0.5*sim)

					severity := SeverityIndirectDependency
					if curr.depth == 0 {
						severity = SeverityDirectDependency
					}

					callerSym := db.symbols[callerID]
					node := &GuidedImpactNode{
						Symbol:          callerSym,
						Severity:        severity,
						GraphDepth:      curr.depth + 1,
						HammingDistance: dist,
						SemanticScore:   sim,
						CombinedRisk:    combinedRisk,
						Reason:          fmt.Sprintf("Dependência reversa estrutural (nível %d) calibrada por afinidade semântica %.2f", curr.depth+1, sim),
					}
					impactMap[callerID] = node

					queue = append(queue, queueItem{id: callerID, depth: curr.depth + 1})
				}
			}
		}
	}

	// 3. Associação Semântica sem Aresta no CSR (Semantic Association)
	if hasNeural {
		for i := 0; i < len(db.symbols); i++ {
			sid := db.symbols[i].ID
			if sid == targetID || visited.Has(sid) {
				continue
			}

			dist, sim, hasVec := getHamming(sid)
			if hasVec && sim >= semanticThreshold {
				node := &GuidedImpactNode{
					Symbol:          db.symbols[sid],
					Severity:        SeveritySemanticAssociation,
					GraphDepth:      -1,
					HammingDistance: dist,
					SemanticScore:   sim,
					CombinedRisk:    0.80 * sim,
					Reason:          fmt.Sprintf("Associação semântica no mesmo domínio funcional (afinidade de %.2f sem chamada no CSR)", sim),
				}
				impactMap[sid] = node
				result.SemanticAssociationCount++
			}
		}
	}

	// 4. Consolidação com ORDENAÇÃO DETERMINÍSTICA ESTÁVEL
	for _, node := range impactMap {
		if node.Severity != SeveritySemanticAssociation {
			result.DeterministicCallersCount++
		}
		result.ImpactedNodes = append(result.ImpactedNodes, node)
	}

	sort.Slice(result.ImpactedNodes, func(i, j int) bool {
		if result.ImpactedNodes[i].CombinedRisk != result.ImpactedNodes[j].CombinedRisk {
			return result.ImpactedNodes[i].CombinedRisk > result.ImpactedNodes[j].CombinedRisk
		}
		return result.ImpactedNodes[i].Symbol.ID < result.ImpactedNodes[j].Symbol.ID
	})

	return result, nil
}
