package hdccsr

// ExpRevCSRGraph armazena as arestas reversas (predecessores) para busca de dependências em O(1).
type ExpRevCSRGraph struct {
	TotalNodes int
	RevOffsets []uint32
	RevEdges   []uint16
}

// BuildRevCSR constrói o índice CSR reverso a partir do CSR direto.
func BuildRevCSR(graph *ExpCSRGraph) *ExpRevCSRGraph {
	total := graph.TotalNodes
	inDegrees := make([]uint32, total)

	for _, tgt := range graph.Edges {
		inDegrees[tgt]++
	}

	revOffsets := make([]uint32, total+1)
	current := uint32(0)
	for i := 0; i < total; i++ {
		revOffsets[i] = current
		current += inDegrees[i]
	}
	revOffsets[total] = current

	revEdges := make([]uint16, len(graph.Edges))
	fillPos := make([]uint32, total)
	copy(fillPos, revOffsets[:total])

	for u := 0; u < total; u++ {
		start := graph.Offsets[u]
		end := graph.Offsets[u+1]
		for e := start; e < end; e++ {
			tgt := graph.Edges[e]
			pos := fillPos[tgt]
			revEdges[pos] = uint16(u)
			fillPos[tgt]++
		}
	}

	return &ExpRevCSRGraph{
		TotalNodes: total,
		RevOffsets: revOffsets,
		RevEdges:   revEdges,
	}
}

// InvalidationSet mantém o conjunto de nós afetados pela mutação de um nó.
type InvalidationSet struct {
	Hop1Affected []uint16 // Nós que chamam diretamente o nó alterado (u -> v)
	Hop2Affected []uint16 // Nós que chamam nós que chamam o alterado (w -> u -> v)
	AllAffected  map[uint16]bool
}

// FindAffectedCone2HopFast localiza o cone de dependência reversa em tempo ótimo via CSR reverso.
// Complexidade: O(grau_reverso_1 + grau_reverso_2), sem varredura global do grafo.
func FindAffectedCone2HopFast(revGraph *ExpRevCSRGraph, mutatedNode uint16) InvalidationSet {
	all := make(map[uint16]bool)
	all[mutatedNode] = true

	// 1. Predecessores de 1-hop (quem chama mutatedNode)
	startM := revGraph.RevOffsets[mutatedNode]
	endM := revGraph.RevOffsets[mutatedNode+1]
	hop1List := make([]uint16, 0, endM-startM)

	for e := startM; e < endM; e++ {
		u := revGraph.RevEdges[e]
		if !all[u] {
			all[u] = true
			hop1List = append(hop1List, u)
		}
	}

	// 2. Predecessores de 2-hop (quem chama os nós de hop1)
	hop2List := make([]uint16, 0, len(hop1List)*4)
	for _, u := range hop1List {
		startU := revGraph.RevOffsets[u]
		endU := revGraph.RevOffsets[u+1]
		for e := startU; e < endU; e++ {
			w := revGraph.RevEdges[e]
			if !all[w] {
				all[w] = true
				hop2List = append(hop2List, w)
			}
		}
	}

	return InvalidationSet{
		Hop1Affected: hop1List,
		Hop2Affected: hop2List,
		AllAffected:  all,
	}
}

// RecomputeNodeHop1 recalcula o vetor contextual de 1-hop para o nó v.
func (s *HDCSystem) RecomputeNodeHop1(graph *ExpCSRGraph, orig []Hypervector, v int) (Hypervector, bool) {
	start := graph.Offsets[v]
	end := graph.Offsets[v+1]
	degree := int(end - start)
	if degree == 0 {
		return Hypervector{}, false
	}

	msgBuf := make([]Hypervector, 0, degree)
	for e := start; e < end; e++ {
		u := graph.Edges[e]
		rel := graph.Relations[e]
		relCfg := s.RelConfigs[rel]
		msg := Bind(RotL8K(orig[u], relCfg.Shift), relCfg.RoleVector)
		msgBuf = append(msgBuf, msg)
	}

	return Majority(msgBuf, s.TieBreaker)
}

// RecomputeNodeHop2 recalcula o vetor contextual de 2-hop para o nó v.
// Se não houver caminhos de 2 saltos (ex: todos os vizinhos são folhas),
// aplica rigorosamente o mesmo fallback da reconstrução completa: preserva o contexto de 1-hop!
func (s *HDCSystem) RecomputeNodeHop2(graph *ExpCSRGraph, orig []Hypervector, v int) (Hypervector, bool) {
	startV := graph.Offsets[v]
	endV := graph.Offsets[v+1]

	if startV == endV {
		return Hypervector{}, false
	}

	msgBuf := make([]Hypervector, 0, 16)

	for eV := startV; eV < endV; eV++ {
		u := graph.Edges[eV]
		rel1 := graph.Relations[eV]
		cfg1 := s.RelConfigs[rel1]

		startU := graph.Offsets[u]
		endU := graph.Offsets[u+1]

		for eU := startU; eU < endU; eU++ {
			w := graph.Edges[eU]
			if int(w) == v {
				continue // Evita backtracking v -> u -> v
			}
			rel2 := graph.Relations[eU]
			cfg2 := s.RelConfigs[rel2]

			roleComposite := Bind(RotL8K(cfg2.RoleVector, cfg1.Shift), cfg1.RoleVector)
			shiftComposite := cfg1.Shift + cfg2.Shift

			msg2Hop := Bind(RotL8K(orig[w], shiftComposite), roleComposite)
			msgBuf = append(msgBuf, msg2Hop)
		}
	}

	if len(msgBuf) == 0 {
		// FALLBACK CANÔNICO IDÊNTICO À RECONSTRUÇÃO COMPLETA:
		// Quando não há caminhos de 2 saltos, preserva o contexto de 1-hop recalculado!
		return s.RecomputeNodeHop1(graph, orig, v)
	}

	return Majority(msgBuf, s.TieBreaker)
}

// UpdateNodeIncremental2Hop atualiza a memória de 2-hop após a mutação do vetor original de um nó.
func (s *HDCSystem) UpdateNodeIncremental2Hop(
	graph *ExpCSRGraph,
	revGraph *ExpRevCSRGraph,
	currentMem *ContextualMemory,
	mutatedNode uint16,
	newVec Hypervector,
) (*ContextualMemory, InvalidationSet) {
	affected := FindAffectedCone2HopFast(revGraph, mutatedNode)

	// Atualiza o vetor original do nó mutado
	currentMem.Original[mutatedNode] = newVec

	// Recalcula o contexto de 2-hop para o próprio nó mutado
	ctxSelf, okSelf := s.RecomputeNodeHop2(graph, currentMem.Original, int(mutatedNode))
	currentMem.Contextual[mutatedNode] = ctxSelf
	currentMem.HasContext[mutatedNode] = okSelf

	// Recalcula o contexto para os nós de 1-hop afetados
	for _, u := range affected.Hop1Affected {
		ctx, ok := s.RecomputeNodeHop2(graph, currentMem.Original, int(u))
		currentMem.Contextual[u] = ctx
		currentMem.HasContext[u] = ok
	}

	// Recalcula o contexto para os nós de 2-hop afetados
	for _, w := range affected.Hop2Affected {
		if w == mutatedNode {
			continue
		}
		ctx, ok := s.RecomputeNodeHop2(graph, currentMem.Original, int(w))
		currentMem.Contextual[w] = ctx
		currentMem.HasContext[w] = ok
	}

	return currentMem, affected
}
