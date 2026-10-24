package hdccsr

import (
	"math/rand"
)

// EdgeRelation define os tipos de relações estruturais no grafo.
type EdgeRelation uint8

const (
	RelCall      EdgeRelation = 1 // Chamada direta: u chama v
	RelRevCall   EdgeRelation = 2 // Chamador inverso: v é chamado por u
	RelMemberOf  EdgeRelation = 3 // Membro estrutural / método
)

// RelationConfig armazena os hipervetores de papel e constantes de rotação de cada relação.
type RelationConfig struct {
	RoleVector Hypervector
	Shift      int
}

// ExpCSRGraph é a estrutura CSR experimental independente para o benchmark HDC.
type ExpCSRGraph struct {
	TotalNodes int
	Offsets    []uint32
	Edges      []uint16
	Relations  []EdgeRelation
}

// NewExpCSRGraph inicializa um grafo CSR experimental.
func NewExpCSRGraph(totalNodes int) *ExpCSRGraph {
	return &ExpCSRGraph{
		TotalNodes: totalNodes,
		Offsets:    make([]uint32, totalNodes+1),
		Edges:      make([]uint16, 0),
		Relations:  make([]EdgeRelation, 0),
	}
}

// BuildCSR constrói o CSR a partir de uma lista de adjacência formatada.
type AdjEdge struct {
	Target uint16
	Rel    EdgeRelation
}

func BuildCSRFromAdj(totalNodes int, adj [][]AdjEdge) *ExpCSRGraph {
	g := NewExpCSRGraph(totalNodes)
	currentOffset := uint32(0)

	for i := 0; i < totalNodes; i++ {
		g.Offsets[i] = currentOffset
		for _, edge := range adj[i] {
			g.Edges = append(g.Edges, edge.Target)
			g.Relations = append(g.Relations, edge.Rel)
			currentOffset++
		}
	}
	g.Offsets[totalNodes] = currentOffset
	return g
}

// HDCSystem gerencia as relações, sementes e a propagação estrutural HDC.
type HDCSystem struct {
	RelConfigs map[EdgeRelation]RelationConfig
	TieBreaker Hypervector
}

// NewHDCSystem cria um sistema HDC com sementes fixas e determinísticas (reproduzibilidade garantida).
func NewHDCSystem(seed int64) *HDCSystem {
	rng := rand.New(rand.NewSource(seed))

	configs := make(map[EdgeRelation]RelationConfig)
	configs[RelCall] = RelationConfig{
		RoleVector: GenerateRandomHypervector(rng),
		Shift:      13, // Deslocamento circular para chamada
	}
	configs[RelRevCall] = RelationConfig{
		RoleVector: GenerateRandomHypervector(rng),
		Shift:      47, // Deslocamento circular para chamada inversa
	}
	configs[RelMemberOf] = RelationConfig{
		RoleVector: GenerateRandomHypervector(rng),
		Shift:      79, // Deslocamento circular para membro
	}

	return &HDCSystem{
		RelConfigs: configs,
		TieBreaker: GenerateRandomHypervector(rng),
	}
}

// ContextualMemory armazena os vetores originais preservados e os vetores contextuais derivados.
type ContextualMemory struct {
	Original   []Hypervector
	Contextual []Hypervector
	HasContext []bool
}

// PropagateHop1 executa 1 camada de passagem de mensagens HDC sobre o CSR:
// m_{u -> v} = ROTL(h_u, k_r) ⊕ R_r
// c_v = MAJORITY({m_{u -> v} : u ∈ N(v)})
func (s *HDCSystem) PropagateHop1(graph *ExpCSRGraph, original []Hypervector) *ContextualMemory {
	total := graph.TotalNodes
	mem := &ContextualMemory{
		Original:   original,
		Contextual: make([]Hypervector, total),
		HasContext: make([]bool, total),
	}

	// Buffer de mensagens reutilizável para minimizar alocações
	msgBuf := make([]Hypervector, 0, 16)

	for v := 0; v < total; v++ {
		start := graph.Offsets[v]
		end := graph.Offsets[v+1]
		degree := int(end - start)

		if degree == 0 {
			mem.HasContext[v] = false
			continue
		}

		msgBuf = msgBuf[:0]
		for e := start; e < end; e++ {
			u := graph.Edges[e]
			rel := graph.Relations[e]

			relCfg := s.RelConfigs[rel]
			// m_{u -> v} = ROTL(h_u, k_r) ⊕ R_r
			msg := Bind(RotL8K(original[u], relCfg.Shift), relCfg.RoleVector)
			msgBuf = append(msgBuf, msg)
		}

		ctxVec, ok := Majority(msgBuf, s.TieBreaker)
		mem.Contextual[v] = ctxVec
		mem.HasContext[v] = ok
	}

	return mem
}

// PropagateHop2 executa 2 camadas de passagem de mensagens HDC:
// A segunda camada agrega mensagens combinando o vetor original com a representação de 1-hop:
// h_{intermediate} = Bind(h_u, c_u^{(1)})
func (s *HDCSystem) PropagateHop2(graph *ExpCSRGraph, hop1 *ContextualMemory) *ContextualMemory {
	total := graph.TotalNodes
	mem := &ContextualMemory{
		Original:   hop1.Original,
		Contextual: make([]Hypervector, total),
		HasContext: make([]bool, total),
	}

	msgBuf := make([]Hypervector, 0, 16)

	for v := 0; v < total; v++ {
		start := graph.Offsets[v]
		end := graph.Offsets[v+1]
		degree := int(end - start)

		if degree == 0 {
			mem.HasContext[v] = false
			continue
		}

		msgBuf = msgBuf[:0]
		for e := start; e < end; e++ {
			u := graph.Edges[e]
			rel := graph.Relations[e]
			relCfg := s.RelConfigs[rel]

			// Representação enriquecida do vizinho no hop 1 (se tiver contexto, une via Bind)
			uRep := hop1.Original[u]
			if hop1.HasContext[u] {
				uRep = Bind(hop1.Original[u], hop1.Contextual[u])
			}

			msg := Bind(RotL8K(uRep, relCfg.Shift), relCfg.RoleVector)
			msgBuf = append(msgBuf, msg)
		}

		ctxVec, ok := Majority(msgBuf, s.TieBreaker)
		mem.Contextual[v] = ctxVec
		mem.HasContext[v] = ok
	}

	return mem
}
