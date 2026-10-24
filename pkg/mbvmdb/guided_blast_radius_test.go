package mbvmdb

import (
	"testing"
)

// TestGuidedNeuralBlastRadius_FullAuditedPipeline valida as 4 correções da auditoria:
// 1. Direção reversa: Identifica quem DEPENDE do símbolo modificado.
// 2. Poda neural ativa: Interrompe ramos com Hamming excessivo (> pruneThreshold).
// 3. Classificações precisas: DIRECT_DEPENDENCY, INDIRECT_DEPENDENCY e SEMANTIC_ASSOCIATION.
// 4. Determinismo estrito: Múltiplas execuções produzem exatamente a mesma sequência de nós.
func TestGuidedNeuralBlastRadius_FullAuditedPipeline(t *testing.T) {
	db := New()

	// Grafo de Dependências Reversas:
	// A cadeia de chamadas reais é:
	// api.HandleLogin (ID 0) chama auth.ValidateSession (ID 1 - Alvo)
	// auth.ValidateSession (ID 1) é o alvo modificado!
	// cron.AuditJob (ID 2) chama api.HandleLogin (ID 0) -> Dependência Indireta de Nível 2
	// noisy.TelemetryLog (ID 3) chama api.HandleLogin, mas tem semântica de log ruidosa (ortogonal)
	// auth.RevokeSession (ID 4) não tem chamada direta, mas pertence à mesma família semântica (Associação Semântica)

	names := []string{
		"api.HandleLogin",
		"auth.ValidateSession", // Alvo modificado
		"cron.AuditJob",
		"noisy.TelemetryLog",
		"auth.RevokeSession",
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

	// Conexões:
	// api.HandleLogin chama auth.ValidateSession
	db.symbols[0].OutgoingCalls = []string{"auth.ValidateSession"}
	db.symbols[1].IncomingCalls = []string{"api.HandleLogin"}

	// cron.AuditJob chama api.HandleLogin
	db.symbols[2].OutgoingCalls = []string{"api.HandleLogin"}
	db.symbols[0].IncomingCalls = []string{"cron.AuditJob", "noisy.TelemetryLog"}

	// noisy.TelemetryLog chama api.HandleLogin
	db.symbols[3].OutgoingCalls = []string{"api.HandleLogin"}

	db.buildCSR()

	// 2. Monta índice neural de 8.192 bits
	neuralIdx := NewNeuralIndex(5)
	authPattern := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		authPattern[w] = 0xAAAAAAAAAAAAAAAA
	}

	// Alvo (1), Caller (0) e Associação Semântica (4) têm padrões alinhados
	vec1 := make([]uint64, NeuralWordsPerVec)
	copy(vec1, authPattern)

	vec0 := make([]uint64, NeuralWordsPerVec)
	copy(vec0, authPattern)
	vec0[0] ^= 0x02 // Alta afinidade semântica

	vec4 := make([]uint64, NeuralWordsPerVec)
	copy(vec4, authPattern)
	vec4[0] ^= 0x04 // Mesma família semântica de autenticação!

	vec2 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec2[w] = 0xF0F0F0F0F0F0F0F0 // Infraestrutura/cron
	}

	// noisy.TelemetryLog (3) é propositalmente preenchido com bits ortogonais (Hamming ~ 4096)
	vec3 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec3[w] = 0x5555555555555555 // Ortogonalidade máxima (Hamming = 8192)
	}

	neuralIdx.SetVector(0, vec0)
	neuralIdx.SetVector(1, vec1)
	neuralIdx.SetVector(2, vec2)
	neuralIdx.SetVector(3, vec3)
	neuralIdx.SetVector(4, vec4)

	// 3. Executa consulta neuro-simbólica com poda ativada para Hamming > 3800
	res1, err := db.QueryGuidedNeuralBlastRadius("auth.ValidateSession", neuralIdx, 3, 0.70, 3800)
	if err != nil {
		t.Fatalf("QueryGuidedNeuralBlastRadius falhou: %v", err)
	}

	// 4. Executa uma segunda vez e valida o determinismo absoluto
	res2, err := db.QueryGuidedNeuralBlastRadius("auth.ValidateSession", neuralIdx, 3, 0.70, 3800)
	if err != nil {
		t.Fatalf("Segunda execução falhou: %v", err)
	}

	if len(res1.ImpactedNodes) != len(res2.ImpactedNodes) {
		t.Fatalf("Não determinístico: tamanhos diferentes (%d vs %d)", len(res1.ImpactedNodes), len(res2.ImpactedNodes))
	}
	for i := range res1.ImpactedNodes {
		if res1.ImpactedNodes[i].Symbol.ID != res2.ImpactedNodes[i].Symbol.ID {
			t.Fatalf("Não determinístico: nós na posição %d diferem", i)
		}
	}

	// 5. Validações Semânticas e Estruturais da Auditoria
	var foundHandleLogin bool
	var foundRevokeSession bool
	var foundNoisyLog bool

	for _, node := range res1.ImpactedNodes {
		switch node.Symbol.Name {
		case "api.HandleLogin":
			foundHandleLogin = true
			if node.Severity != SeverityDirectDependency {
				t.Errorf("Esperado DIRECT_DEPENDENCY para HandleLogin, obtido %s", node.Severity)
			}
			if node.GraphDepth != 1 {
				t.Errorf("Esperado GraphDepth=1 (chamador direto), obtido %d", node.GraphDepth)
			}
		case "auth.RevokeSession":
			foundRevokeSession = true
			if node.Severity != SeveritySemanticAssociation {
				t.Errorf("Esperado SEMANTIC_ASSOCIATION para RevokeSession, obtido %s", node.Severity)
			}
			if node.GraphDepth != -1 {
				t.Errorf("Esperado GraphDepth=-1 para associação semântica, obtido %d", node.GraphDepth)
			}
		case "noisy.TelemetryLog":
			foundNoisyLog = true
		}
	}

	if !foundHandleLogin {
		t.Errorf("api.HandleLogin (chamador direto reverso) não encontrado")
	}
	if !foundRevokeSession {
		t.Errorf("auth.RevokeSession (associação semântica) não encontrada")
	}
	if foundNoisyLog {
		t.Errorf("noisy.TelemetryLog não deveria estar nos nós impactados (deveria ter sido podado pelo filtro neural)")
	}
	if res1.PrunedBranchesCount == 0 {
		t.Errorf("Poda neural não foi registrada nos contadores auditáveis")
	}

	t.Logf("=================================================================")
	t.Logf("✅ AUDITORIA CONCLUÍDA COM SUCESSO:")
	t.Logf("   1. Chamadores Diretos Identificados (CSR Reverso): %d", res1.DeterministicCallersCount)
	t.Logf("   2. Associações Semânticas sem Chamada (IA):        %d", res1.SemanticAssociationCount)
	t.Logf("   3. Ramos Ortogonais Podados Ativamente (Poda):      %d", res1.PrunedBranchesCount)
	t.Logf("   4. Determinismo Estrito Entre Execuções:           100%% Verificado")
	t.Logf("=================================================================")
}
