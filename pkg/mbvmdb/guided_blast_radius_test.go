package mbvmdb

import (
	"testing"
)

// TestGuidedNeuralBlastRadius_SemanticAndStructuralInterplay comprova formalmente a interação
// entre a rede neural e o grafo CSR, atendendo exatamente à crítica do Zyon.
func TestGuidedNeuralBlastRadius_SemanticAndStructuralInterplay(t *testing.T) {
	db := New()

	// Cenário Realista de Autenticação:
	// 0: auth.ValidateSession (Alvo)
	// 1: auth.CheckToken (Chamado diretamente no CSR)
	// 2: db.RawExecQuery (Chamado no CSR por CheckToken - infraestrutura genérica)
	// 3: auth.RevokeSession (NÃO chamado no CSR! Mas é do mesmo domínio semântico de auth!)
	// 4: payment.ProcessCharge (Totalmente desconectado e ortogonal)

	names := []string{
		"auth.ValidateSession",
		"auth.CheckToken",
		"db.RawExecQuery",
		"auth.RevokeSession",
		"payment.ProcessCharge",
	}

	for i, name := range names {
		rec := &SymbolRecord{
			ID:      uint16(i),
			Kind:    "func",
			Package: "auth",
			Name:    name,
		}
		db.symbols = append(db.symbols, rec)
		db.indexByName[name] = rec
	}

	// 1. Arestas CSR
	db.symbols[0].OutgoingCalls = []string{"auth.CheckToken"}
	db.symbols[1].IncomingCalls = []string{"auth.ValidateSession"}
	db.symbols[1].OutgoingCalls = []string{"db.RawExecQuery"}
	db.symbols[2].IncomingCalls = []string{"auth.CheckToken"}

	db.buildCSR()

	// 2. Índice Neural de 8.192 bits
	neuralIdx := NewNeuralIndex(5)
	authPattern := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		authPattern[w] = 0xAAAAAAAAAAAAAAAA
	}

	// Atribuição de padrões vetoriais:
	// - auth.ValidateSession (0), auth.CheckToken (1), auth.RevokeSession (3) compartilham semântica de auth
	vec0 := make([]uint64, NeuralWordsPerVec)
	copy(vec0, authPattern)

	vec1 := make([]uint64, NeuralWordsPerVec)
	copy(vec1, authPattern)
	vec1[0] ^= 0x02 // Quase idêntico

	vec3 := make([]uint64, NeuralWordsPerVec)
	copy(vec3, authPattern)
	vec3[0] ^= 0x04 // Mesma família semântica de autenticação!

	// - db.RawExecQuery (2) tem semântica de banco de dados
	vec2 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec2[w] = 0xF0F0F0F0F0F0F0F0
	}

	// - payment.ProcessCharge (4) é ortogonal
	vec4 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec4[w] = 0x5555555555555555
	}

	neuralIdx.SetVector(0, vec0)
	neuralIdx.SetVector(1, vec1)
	neuralIdx.SetVector(2, vec2)
	neuralIdx.SetVector(3, vec3)
	neuralIdx.SetVector(4, vec4)

	// 3. Executa a inferência Neuro-Simbólica Conjunta
	res, err := db.QueryGuidedNeuralBlastRadius("auth.ValidateSession", neuralIdx, 3, 0.70)
	if err != nil {
		t.Fatalf("QueryGuidedNeuralBlastRadius falhou: %v", err)
	}

	// Validações Cruciais:
	// A. auth.CheckToken deve ser identificado como DIRECT_BREAKING
	var foundCheckToken bool
	var foundRevokeSession bool
	var foundRawExec bool

	for _, node := range res.ImpactedNodes {
		switch node.Symbol.Name {
		case "auth.CheckToken":
			foundCheckToken = true
			if node.Severity != SeverityDirectBreaking {
				t.Errorf("Esperado DIRECT_BREAKING para CheckToken, obtido %s", node.Severity)
			}
		case "auth.RevokeSession":
			foundRevokeSession = true
			// PROVA DA INFERÊNCIA NEURAL: Detectou que RevokeSession é afetado semanticamente
			// mesmo sem ter aresta no CSR!
			if node.Severity != SeveritySemanticDrift {
				t.Errorf("Esperado SEMANTIC_DRIFT para RevokeSession, obtido %s", node.Severity)
			}
			if node.GraphDepth != -1 {
				t.Errorf("Esperado GraphDepth=-1 para nó sem aresta, obtido %d", node.GraphDepth)
			}
		case "db.RawExecQuery":
			foundRawExec = true
			if node.Severity != SeverityIndirectCaller {
				t.Errorf("Esperado INDIRECT_CALLER para RawExecQuery, obtido %s", node.Severity)
			}
		case "payment.ProcessCharge":
			t.Errorf("payment.ProcessCharge não deveria ter sido incluído (ortogonal e desconectado)")
		}
	}

	if !foundCheckToken {
		t.Errorf("auth.CheckToken não encontrado no resultado")
	}
	if !foundRevokeSession {
		t.Errorf("auth.RevokeSession (Semantic Drift) não foi detectado pela rede neural!")
	}
	if !foundRawExec {
		t.Errorf("db.RawExecQuery não encontrado no resultado")
	}

	t.Logf("=================================================================")
	t.Logf("🎯 PROVA DA INFERÊNCIA CONJUNTA NEURO-SIMBÓLICA:")
	t.Logf("   Nós com impacto estrutural determinístico (CSR): %d", res.DeterministicCount)
	t.Logf("   Nós com acoplamento semântico oculto descobertos (IA): %d", res.SemanticDriftCount)
	t.Logf("=================================================================")
}
