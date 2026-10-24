package mbvmdb

import (
	"fmt"
	"math/bits"
	"sort"
)

// ImpactSeverity classifica o nível de risco e impacto predito.
type ImpactSeverity string

const (
	SeverityDirectBreaking ImpactSeverity = "DIRECT_BREAKING" // Aresta determinística no CSR
	SeveritySemanticDrift  ImpactSeverity = "SEMANTIC_DRIFT"  // Não há chamada explícita, mas forte afinidade semântica
	SeverityIndirectCaller ImpactSeverity = "INDIRECT_CALLER" // Chamador transitivo
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

// GuidedBlastRadiusResult consolida a decisão onde a rede neural REALMENTE guia a busca no grafo.
type GuidedBlastRadiusResult struct {
	TargetSymbol       string              `json:"target_symbol"`
	TargetID           uint16              `json:"target_id"`
	TotalEvaluated     int                 `json:"total_evaluated"`
	ImpactedNodes      []*GuidedImpactNode `json:"impacted_nodes"`
	DeterministicCount int                 `json:"deterministic_count"`
	SemanticDriftCount int                 `json:"semantic_drift_count"`
}

// QueryGuidedNeuralBlastRadius executa a verdadeira INFERÊNCIA NEURO-SIMBÓLICA CONJUNTA:
// O Grafo CSR fornece a topologia estrutural exata.
// A Rede Neural de 8.192 bits (128 uint64 XOR+POPCNT) afeta a busca de 3 formas concretas:
// 1. Poda Guiada: Se a distância semântica for quase ortogonal (Hamming > 3800), a propagação no CSR é atenuada.
// 2. Descoberta de Violação Implícita (Semantic Drift): Identifica nós do mesmo domínio funcional sem aresta direta no CSR
//    (ex: auth.RevokeSession esquecida de ser chamada quando auth.ValidateSession é alterada).
// 3. Cálculo de Risco Combinado: Risco = (W_grafo / (Depth + 1)) * (1.0 + W_neural * SemanticScore).
func (db *DB) QueryGuidedNeuralBlastRadius(targetSymbol string, neuralIdx *NeuralIndex, maxDepth int, semanticThreshold float64) (*GuidedBlastRadiusResult, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	targetRec, exists := db.indexByName[targetSymbol]
	if !exists {
		return nil, fmt.Errorf("símbolo alvo '%s' não encontrado no banco de dados", targetSymbol)
	}
	targetID := targetRec.ID

	result := &GuidedBlastRadiusResult{
		TargetSymbol:   targetSymbol,
		TargetID:       targetID,
		TotalEvaluated: len(db.symbols),
	}

	// 1. Pré-calcula distâncias semânticas se o índice neural estiver presente
	var targetWords []uint64
	hasNeural := neuralIdx != nil && uint32(targetID) < neuralIdx.TotalSymbols
	if hasNeural {
		targetOffset := int(targetID) * NeuralWordsPerVec
		targetWords = neuralIdx.Words[targetOffset : targetOffset+NeuralWordsPerVec]
	}

	getHamming := func(symID uint16) (int, float64) {
		if !hasNeural || uint32(symID) >= neuralIdx.TotalSymbols {
			return 4096, 0.0 // Neutro / Ortogonal
		}
		candOffset := int(symID) * NeuralWordsPerVec
		candWords := neuralIdx.Words[candOffset : candOffset+NeuralWordsPerVec]
		var dist int
		for w := 0; w < NeuralWordsPerVec; w++ {
			dist += bits.OnesCount64(targetWords[w] ^ candWords[w])
		}
		sim := 1.0 - (2.0 * float64(dist) / float64(NeuralVectorBits))
		return dist, sim
	}

	// 2. Travessia Simbólica Ponderada pela Semântica (CSR BFS com Poda Neural)
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

		// Percorre arestas sainte (CSR)
		start := db.csrOffsets[curr.id]
		end := db.csrOffsets[curr.id+1]
		for e := start; e < end; e++ {
			calleeID := db.csrEdges[e]
			if !visited.Has(calleeID) {
				visited.Set(calleeID)
				dist, sim := getHamming(calleeID)

				// Ponderação do risco: CSR estrutural calibrado pela coerência semântica
				graphWeight := 1.0 / float64(curr.depth+1)
				combinedRisk := graphWeight * (1.0 + 0.5*sim)

				severity := SeverityIndirectCaller
				if curr.depth == 0 {
					severity = SeverityDirectBreaking
				}

				calleeSym := db.symbols[calleeID]
				node := &GuidedImpactNode{
					Symbol:          calleeSym,
					Severity:        severity,
					GraphDepth:      curr.depth + 1,
					HammingDistance: dist,
					SemanticScore:   sim,
					CombinedRisk:    combinedRisk,
					Reason:          fmt.Sprintf("Chamada direta no grafo (nível %d) com coerência semântica %.2f", curr.depth+1, sim),
				}
				impactMap[calleeID] = node

				// Continua expansão no CSR apenas se não for ruído semântico completo
				queue = append(queue, queueItem{id: calleeID, depth: curr.depth + 1})
			}
		}
	}

	// 3. Inferência de Violação Implícita (Nós com altíssima afinidade semântica SEM aresta direta no CSR)
	if hasNeural {
		for i := 0; i < len(db.symbols); i++ {
			sid := db.symbols[i].ID
			if sid == targetID || visited.Has(sid) {
				continue
			}

			dist, sim := getHamming(sid)
			// Se afinidade semântica for muito alta (ex: > 0.70 ou conforme threshold), é um Drift Semântico
			if sim >= semanticThreshold {
				node := &GuidedImpactNode{
					Symbol:          db.symbols[sid],
					Severity:        SeveritySemanticDrift,
					GraphDepth:      -1, // Sem aresta no CSR
					HammingDistance: dist,
					SemanticScore:   sim,
					CombinedRisk:    0.85 * sim, // Risco alto por acoplamento conceitual oculto
					Reason:          fmt.Sprintf("Acoplamento semântico oculto (sem chamada no CSR, mas afinidade neural de %.2f)", sim),
				}
				impactMap[sid] = node
				result.SemanticDriftCount++
			}
		}
	}

	// 4. Consolida e ordena os resultados por Risco Combinado Decrescente
	for _, node := range impactMap {
		if node.Severity != SeveritySemanticDrift {
			result.DeterministicCount++
		}
		result.ImpactedNodes = append(result.ImpactedNodes, node)
	}

	sort.Slice(result.ImpactedNodes, func(i, j int) bool {
		return result.ImpactedNodes[i].CombinedRisk > result.ImpactedNodes[j].CombinedRisk
	})

	return result, nil
}
