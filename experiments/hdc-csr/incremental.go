package hdccsr

// InvalidationSet mantém o conjunto de nós afetados pela mutação de um nó.
type InvalidationSet struct {
	Hop1Affected []uint16 // Nós que chamam diretamente o nó alterado (u -> v)
	Hop2Affected []uint16 // Nós que chamam nós que chamam o alterado (w -> u -> v)
	AllAffected  map[uint16]bool
}

// FindAffectedCone2Hop identifica todos os nós cuja memória de 2 saltos depende do mutatedNode.
// Para isso, navega as arestas reversas (quem tem aresta apontando para mutatedNode).
func FindAffectedCone2Hop(graph *ExpCSRGraph, mutatedNode uint16) InvalidationSet {
	// 1. Encontra predecessores de 1-hop (u -> mutatedNode)
	// Como graph armazena arestas de saída, construímos a lista reversa ou varremos:
	total := graph.TotalNodes
	hop1Set := make(map[uint16]bool)

	for u := 0; u < total; u++ {
		start := graph.Offsets[u]
		end := graph.Offsets[u+1]
		for e := start; e < end; e++ {
			if graph.Edges[e] == mutatedNode {
				hop1Set[uint16(u)] = true
				break
			}
		}
	}

	// 2. Encontra predecessores de 2-hop (w -> u)
	hop2Set := make(map[uint16]bool)
	for w := 0; w < total; w++ {
		start := graph.Offsets[w]
		end := graph.Offsets[w+1]
		for e := start; e < end; e++ {
			target := graph.Edges[e]
			if hop1Set[target] {
				hop2Set[uint16(w)] = true
				break
			}
		}
	}

	all := make(map[uint16]bool)
	all[mutatedNode] = true

	hop1List := make([]uint16, 0, len(hop1Set))
	for u := range hop1Set {
		hop1List = append(hop1List, u)
		all[u] = true
	}

	hop2List := make([]uint16, 0, len(hop2Set))
	for w := range hop2Set {
		hop2List = append(hop2List, w)
		all[w] = true
	}

	return InvalidationSet{
		Hop1Affected: hop1List,
		Hop2Affected: hop2List,
		AllAffected:  all,
	}
}

// RecomputeNodeHop2 recalcula o vetor contextual de 2-hop de um único nó v no CSR.
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
				continue // Evita backtracking imediato v -> u -> v
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
		return Hypervector{}, false
	}

	return Majority(msgBuf, s.TieBreaker)
}

// UpdateNodeIncremental2Hop atualiza a memória de 2-hop após a mutação do vetor original de um nó.
// Atualiza estritamente os nós que dependem do nó alterado, preservando todos os demais.
func (s *HDCSystem) UpdateNodeIncremental2Hop(
	graph *ExpCSRGraph,
	currentMem *ContextualMemory,
	mutatedNode uint16,
	newVec Hypervector,
) (*ContextualMemory, InvalidationSet) {
	// Identifica o cone afetado
	affected := FindAffectedCone2Hop(graph, mutatedNode)

	// Atualiza o vetor original do nó mutado
	currentMem.Original[mutatedNode] = newVec

	// Recalcula o contexto de 2-hop para o nó mutado (se tiver caminhos de saída)
	ctxSelf, okSelf := s.RecomputeNodeHop2(graph, currentMem.Original, int(mutatedNode))
	currentMem.Contextual[mutatedNode] = ctxSelf
	currentMem.HasContext[mutatedNode] = okSelf

	// Recalcula o contexto apenas para os predecessores afetados (Hop1 e Hop2)
	for _, u := range affected.Hop1Affected {
		ctx, ok := s.RecomputeNodeHop2(graph, currentMem.Original, int(u))
		currentMem.Contextual[u] = ctx
		currentMem.HasContext[u] = ok
	}

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
