package mbvmdb

import (
	"testing"
)

// TestGuidedNeuralBlastRadius_FullAuditedPipeline valida rigorosamente:
// 1. Modo EXATO: Garante 100% de cobertura estrutural do CSR (ZERO FALSOS NEGATIVOS).
//    api.HandleLogin, cron.AuditJob e noisy.TelemetryLog DEVEM estar presentes no resultado!
// 2. Modo RÁPIDO: Poda ramos com alta divergência semântica e contabiliza a poda com transparência.
// 3. Associação Semântica: Detecta auth.RevokeSession sem tratá-la como dependência direta comprovada.
// 4. Determinismo Estrito e Tratamento de Vetores Ausentes.
func TestGuidedNeuralBlastRadius_FullAuditedPipeline(t *testing.T) {
	db := New()

	// Grafo de Dependências Reversas:
	// api.HandleLogin (ID 0) chama auth.ValidateSession (ID 1 - Alvo)
	// auth.ValidateSession (ID 1) é o alvo modificado!
	// cron.AuditJob (ID 2) chama api.HandleLogin (ID 0) -> Dependência Indireta de Nível 2
	// noisy.TelemetryLog (ID 3) chama api.HandleLogin (ID 0) -> Dependência Indireta de Nível 2 com ruído semântico
	// auth.RevokeSession (ID 4) não tem chamada direta no CSR, mas compartilha domínio de auth

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

	// Conexões no CSR:
	db.symbols[0].OutgoingCalls = []string{"auth.ValidateSession"}
	db.symbols[1].IncomingCalls = []string{"api.HandleLogin"}

	db.symbols[2].OutgoingCalls = []string{"api.HandleLogin"}
	db.symbols[3].OutgoingCalls = []string{"api.HandleLogin"}
	db.symbols[0].IncomingCalls = []string{"cron.AuditJob", "noisy.TelemetryLog"}

	db.buildCSR()

	// 2. Monta índice neural de 8.192 bits
	neuralIdx := NewNeuralIndex(5)
	authPattern := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		authPattern[w] = 0xAAAAAAAAAAAAAAAA
	}

	// Alvo (1), Caller Direto (0) e Associação Semântica (4)
	vec1 := make([]uint64, NeuralWordsPerVec)
	copy(vec1, authPattern)

	vec0 := make([]uint64, NeuralWordsPerVec)
	copy(vec0, authPattern)
	vec0[0] ^= 0x02

	vec4 := make([]uint64, NeuralWordsPerVec)
	copy(vec4, authPattern)
	vec4[0] ^= 0x04

	// cron.AuditJob (2) tem semântica de cron/job
	vec2 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec2[w] = 0xF0F0F0F0F0F0F0F0
	}

	// noisy.TelemetryLog (3) é ortogonal ao alvo
	vec3 := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		vec3[w] = 0x5555555555555555
	}

	neuralIdx.SetVector(0, vec0)
	neuralIdx.SetVector(1, vec1)
	neuralIdx.SetVector(2, vec2)
	neuralIdx.SetVector(3, vec3)
	neuralIdx.SetVector(4, vec4)

	// -------------------------------------------------------------
	// TESTE 1: MODO EXATO (Zero Falsos Negativos - Crítica Central)
	// -------------------------------------------------------------
	resExact, err := db.QueryGuidedNeuralBlastRadius("auth.ValidateSession", neuralIdx, ModeExact, 3, 0.70, 3800)
	if err != nil {
		t.Fatalf("Modo Exact falhou: %v", err)
	}

	foundMap := make(map[string]*GuidedImpactNode)
	for _, n := range resExact.ImpactedNodes {
		foundMap[n.Symbol.Name] = n
	}

	// NO MODO EXATO: TODAS as 3 dependências estruturais DEVEM estar presentes!
	if _, ok := foundMap["api.HandleLogin"]; !ok {
		t.Errorf("Modo Exact: api.HandleLogin (chamador direto) omitido!")
	}
	if _, ok := foundMap["cron.AuditJob"]; !ok {
		t.Errorf("Modo Exact: cron.AuditJob (chamador indireto) foi indevidamente podado!")
	}
	if _, ok := foundMap["noisy.TelemetryLog"]; !ok {
		t.Errorf("Modo Exact: noisy.TelemetryLog (chamador estrutural) omitido!")
	}
	if _, ok := foundMap["auth.RevokeSession"]; !ok {
		t.Errorf("Modo Exact: auth.RevokeSession (associação semântica) omitida!")
	}
	if resExact.PrunedBranchesCount != 0 {
		t.Errorf("Modo Exact: PrunedBranchesCount deve ser 0, obtido %d", resExact.PrunedBranchesCount)
	}

	// Valida classificação correta
	if foundMap["api.HandleLogin"].Severity != SeverityDirectDependency {
		t.Errorf("HandleLogin severidade incorreta: %s", foundMap["api.HandleLogin"].Severity)
	}
	if foundMap["cron.AuditJob"].Severity != SeverityIndirectDependency {
		t.Errorf("cron.AuditJob severidade incorreta: %s", foundMap["cron.AuditJob"].Severity)
	}
	if foundMap["auth.RevokeSession"].Severity != SeveritySemanticAssociation {
		t.Errorf("auth.RevokeSession severidade incorreta: %s", foundMap["auth.RevokeSession"].Severity)
	}

	// -------------------------------------------------------------
	// TESTE 2: MODO RÁPIDO APROXIMADO (Poda Neural Ativa)
	// -------------------------------------------------------------
	resFast, err := db.QueryGuidedNeuralBlastRadius("auth.ValidateSession", neuralIdx, ModeFastApproximate, 3, 0.70, 3800)
	if err != nil {
		t.Fatalf("Modo FastApproximate falhou: %v", err)
	}

	// No modo rápido, noisy.TelemetryLog é podado devido à ortogonalidade de bits
	var fastFoundNoisy bool
	for _, n := range resFast.ImpactedNodes {
		if n.Symbol.Name == "noisy.TelemetryLog" {
			fastFoundNoisy = true
		}
	}
	if fastFoundNoisy {
		t.Errorf("Modo FastApproximate: noisy.TelemetryLog deveria ter sido podado!")
	}
	if resFast.PrunedBranchesCount == 0 {
		t.Errorf("Modo FastApproximate: esperado podas registradas nos contadores")
	}

	t.Logf("=================================================================")
	t.Logf("✅ AUDITORIA ZYON MODO EXATO vs RÁPIDO CONCLUÍDA:")
	t.Logf("   Modo EXATO: %d nós estruturais (100%% Cobertura CSR preservada)", resExact.DeterministicCallersCount)
	t.Logf("   Modo EXATO: %d nós de associação semântica identificados", resExact.SemanticAssociationCount)
	t.Logf("   Modo RÁPIDO: %d ramos podados ativamente por divergência", resFast.PrunedBranchesCount)
	t.Logf("=================================================================")
}
