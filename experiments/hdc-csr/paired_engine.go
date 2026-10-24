package hdccsr

import (
	"sort"
)

// QueryRequest padroniza a entrada idêntica para todos os motores.
type QueryRequest struct {
	SourceID uint16
	Semantic Hypervector
	K        int
}

// QueryResult padroniza a saída idêntica de cada motor.
type QueryResult struct {
	NodeID uint16
	Score  float64
}

// UnifiedEngine define a interface unificada que todos os motores devem satisfazer.
type UnifiedEngine interface {
	Name() string
	QueryTopK(req QueryRequest) []QueryResult
}

// ScoreSemanticOnly calcula a similaridade normalizada [0..1] baseada em Hamming puro.
func ScoreSemanticOnly(query Hypervector, target Hypervector) float64 {
	dist := HammingDistance(query, target)
	return 1.0 - (float64(dist) / float64(VectorBits))
}

// ScoreCombined calcula a similaridade combinada semântica e contextual.
func ScoreCombined(queryOrig, queryCtx, targetOrig, targetCtx Hypervector, hasTargetCtx bool, alpha float64) float64 {
	semScore := ScoreSemanticOnly(queryOrig, targetOrig)
	if !hasTargetCtx {
		return semScore
	}
	ctxDist := HammingDistance(queryCtx, targetCtx)
	ctxScore := 1.0 - (float64(ctxDist) / float64(VectorBits))
	return (1.0-alpha)*semScore + alpha*ctxScore
}

// ---------------------------------------------------------
// 1. Motor CSR + Filtro Semântico (Baseline Estrutural Exato)
// ---------------------------------------------------------
type CSREngine struct {
	graph   *ExpCSRGraph
	origVec []Hypervector
}

func NewCSREngine(g *ExpCSRGraph, orig []Hypervector) *CSREngine {
	return &CSREngine{graph: g, origVec: orig}
}

func (e *CSREngine) Name() string {
	return "CSR_Local_Semantic"
}

func (e *CSREngine) QueryTopK(req QueryRequest) []QueryResult {
	source := req.SourceID
	startV := e.graph.Offsets[source]
	endV := e.graph.Offsets[source+1]

	visited := make(map[uint16]bool)
	candidates := make([]QueryResult, 0, 16)

	// Varre vizinhos de 1-hop e 2-hop no CSR
	for eV := startV; eV < endV; eV++ {
		u := e.graph.Edges[eV]
		if u != source && !visited[u] {
			visited[u] = true
			score := ScoreSemanticOnly(req.Semantic, e.origVec[u])
			candidates = append(candidates, QueryResult{NodeID: u, Score: score})
		}

		startU := e.graph.Offsets[u]
		endU := e.graph.Offsets[u+1]
		for eU := startU; eU < endU; eU++ {
			w := e.graph.Edges[eU]
			if w != source && !visited[w] {
				visited[w] = true
				score := ScoreSemanticOnly(req.Semantic, e.origVec[w])
				candidates = append(candidates, QueryResult{NodeID: w, Score: score})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > req.K {
		candidates = candidates[:req.K]
	}
	return candidates
}

// ---------------------------------------------------------
// 2. Motor Semântico Puro (Baseline Vetorial Isolado)
// ---------------------------------------------------------
type PureSemanticEngine struct {
	totalNodes int
	origVec    []Hypervector
}

func NewPureSemanticEngine(total int, orig []Hypervector) *PureSemanticEngine {
	return &PureSemanticEngine{totalNodes: total, origVec: orig}
}

func (e *PureSemanticEngine) Name() string {
	return "Pure_Semantic_Scan"
}

func (e *PureSemanticEngine) QueryTopK(req QueryRequest) []QueryResult {
	candidates := make([]QueryResult, 0, e.totalNodes-1)

	for i := 0; i < e.totalNodes; i++ {
		id := uint16(i)
		if id == req.SourceID {
			continue
		}
		score := ScoreSemanticOnly(req.Semantic, e.origVec[id])
		candidates = append(candidates, QueryResult{NodeID: id, Score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > req.K {
		candidates = candidates[:req.K]
	}
	return candidates
}

// ---------------------------------------------------------
// 3. Motor HDC Híbrido Pré-computado (Memória Contextual 2-Hop)
// ---------------------------------------------------------
type HDCHybridEngine struct {
	totalNodes int
	origVec    []Hypervector
	contextMem *ContextualMemory
	alpha      float64
}

func NewHDCHybridEngine(total int, orig []Hypervector, ctx *ContextualMemory, alpha float64) *HDCHybridEngine {
	return &HDCHybridEngine{
		totalNodes: total,
		origVec:    orig,
		contextMem: ctx,
		alpha:      alpha,
	}
}

func (e *HDCHybridEngine) Name() string {
	return "HDC_Hybrid_Scan"
}

func (e *HDCHybridEngine) QueryTopK(req QueryRequest) []QueryResult {
	sourceCtx := e.contextMem.Contextual[req.SourceID]
	candidates := make([]QueryResult, 0, e.totalNodes-1)

	for i := 0; i < e.totalNodes; i++ {
		id := uint16(i)
		if id == req.SourceID {
			continue
		}
		score := ScoreCombined(req.Semantic, sourceCtx, e.origVec[id], e.contextMem.Contextual[id], e.contextMem.HasContext[id], e.alpha)
		candidates = append(candidates, QueryResult{NodeID: id, Score: score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	if len(candidates) > req.K {
		candidates = candidates[:req.K]
	}
	return candidates
}
