package hdccsr

import (
	"math/rand"
	"sort"
	"testing"
)

type RetrievalCandidate struct {
	ID    int
	Score float64 // Normalizada [0..1]
}

func scoreSemanticOnly(query Hypervector, target Hypervector) float64 {
	dist := HammingDistance(query, target)
	return 1.0 - (float64(dist) / float64(VectorBits))
}

func scoreCombined(queryOrig, queryCtx, targetOrig, targetCtx Hypervector, hasTargetCtx bool, alpha float64) float64 {
	semScore := scoreSemanticOnly(queryOrig, targetOrig)
	if !hasTargetCtx {
		return semScore
	}
	ctxDist := HammingDistance(queryCtx, targetCtx)
	ctxScore := 1.0 - (float64(ctxDist) / float64(VectorBits))
	return (1.0-alpha)*semScore + alpha*ctxScore
}

func TestExperimentsA_B_C(t *testing.T) {
	const totalNodes = 100
	rng := rand.New(rand.NewSource(999))

	// 1. Gera vetores semânticos originais aleatórios (8.192 bits cada)
	origVectors := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		origVectors[i] = generateRandomHypervector(rng)
	}

	// 2. Constrói grafo sintético de chamadas de código
	// Nó 10 chama Nós 11, 12, 13 (Módulo Autenticação)
	// Nó 20 chama Nós 21, 22 (Módulo Faturamento)
	// Nó 99 é isolado (grau 0)
	adj := make([][]AdjEdge, totalNodes)
	adj[10] = []AdjEdge{
		{Target: 11, Rel: RelCall},
		{Target: 12, Rel: RelCall},
		{Target: 13, Rel: RelCall},
	}
	adj[11] = []AdjEdge{{Target: 10, Rel: RelRevCall}}
	adj[12] = []AdjEdge{{Target: 10, Rel: RelRevCall}}
	adj[13] = []AdjEdge{{Target: 10, Rel: RelRevCall}}

	adj[20] = []AdjEdge{
		{Target: 21, Rel: RelCall},
		{Target: 22, Rel: RelCall},
	}
	adj[21] = []AdjEdge{{Target: 20, Rel: RelRevCall}}
	adj[22] = []AdjEdge{{Target: 20, Rel: RelRevCall}}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(12345)

	// Propagação Camada 1 (Experimento B)
	memHop1 := hdc.PropagateHop1(graph, origVectors)

	// Propagação Camada 2 (Experimento C)
	memHop2 := hdc.PropagateHop2(graph, memHop1)

	// Validação Hop 2: Nós folhas no Hop 2 continuam tendo contexto enriquecido
	if !memHop2.HasContext[11] || !memHop2.HasContext[10] {
		t.Fatalf("Camada 2 deveria propagar contexto para nós centrais e folhas")
	}

	// Validação 1: Nó isolado preserva hasContext = false
	if memHop1.HasContext[99] {
		t.Fatalf("Nó 99 isolado deveria ter HasContext=false")
	}

	// Validação 2: Nós com 1 vizinho (11, 12, 13) adquiriram contexto
	for _, leaf := range []int{11, 12, 13} {
		if !memHop1.HasContext[leaf] {
			t.Fatalf("Nó folha %d deveria ter HasContext=true", leaf)
		}
		// A mensagem recebida do nó 10 deve refletir a chamada inversa
		expectedMsg := Bind(RotL8K(origVectors[10], hdc.RelConfigs[RelRevCall].Shift), hdc.RelConfigs[RelRevCall].RoleVector)
		if memHop1.Contextual[leaf] != expectedMsg {
			t.Fatalf("Nó folha %d deveria conter exatamente a mensagem recebida de seu único vizinho", leaf)
		}
	}

	// Validação 3: Teste de similaridade contextual entre nós do mesmo módulo vs módulo diferente
	// Nós 11 e 12 compartilham o mesmo chamador (Nó 10) com a mesma relação (RelRevCall).
	// Portanto seus vetores contextuais DEVEM ser idênticos ou muito próximos!
	ctxDist11_12 := HammingDistance(memHop1.Contextual[11], memHop1.Contextual[12])
	if ctxDist11_12 != 0 {
		t.Fatalf("Nós 11 e 12 recebem a mesma mensagem do nó 10, logo ctxDist deveria ser 0, obtido %d", ctxDist11_12)
	}

	// Enquanto a distância contextual entre o Nó 11 (Autenticação) e Nó 21 (Faturamento) deve ser quase-ortogonal (~4.096)
	ctxDist11_21 := HammingDistance(memHop1.Contextual[11], memHop1.Contextual[21])
	if ctxDist11_21 < 3800 || ctxDist11_21 > 4400 {
		t.Fatalf("Distância contextual entre módulos distintos esperada ~4096, obtido %d", ctxDist11_21)
	}

	// Validação 4: Comparação de Ranking
	// Simulando uma busca estrutural onde o usuário procura contexto do "Nó 11":
	// No Experimento A (sem HDC), 12 e 13 não têm nenhuma correlação semântica pura a priori.
	// No Experimento B (com HDC alpha=0.5), Nós 12 e 13 são alavancados ao topo do ranking!
	candidates := make([]RetrievalCandidate, totalNodes)
	for i := 0; i < totalNodes; i++ {
		candidates[i] = RetrievalCandidate{
			ID:    i,
			Score: scoreCombined(origVectors[11], memHop1.Contextual[11], origVectors[i], memHop1.Contextual[i], memHop1.HasContext[i], 0.5),
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})

	// O próprio nó 11 é top-1 (score 1.0)
	if candidates[0].ID != 11 {
		t.Errorf("Top 1 deveria ser o próprio nó 11")
	}
	// O nó 12 ou 13 devem estar no Top 3 devido ao contexto compartilhado idêntico
	top3IDs := map[int]bool{candidates[0].ID: true, candidates[1].ID: true, candidates[2].ID: true}
	if !top3IDs[12] && !top3IDs[13] {
		t.Errorf("Nós irmãos estruturais (12, 13) deveriam estar no topo pelo score combinado HDC")
	}

	t.Logf("Experimento A, B e C concluídos com sucesso. Separação modular verificada.")
}
