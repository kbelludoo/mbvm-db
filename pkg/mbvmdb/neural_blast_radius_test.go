package mbvmdb

import (
	"fmt"
	"math/rand"
	"testing"
)

// BenchmarkBlastRadiusEndToEnd executa a consulta completa de ponta a ponta recomendada pelo Zyon:
// - Busca de símbolo em grafo
// - Varredura neural de 8.192 bits (128 uint64 XOR + POPCNT) entre todos os nós
// - Filtro dos Top-30 candidatos
// - Travessia CSR de Blast Radius (BFS com Bitset de 64 bits)
func BenchmarkBlastRadiusEndToEnd(b *testing.B) {
	rand.Seed(42)

	const totalNodes = 1000
	db := New()

	for i := 0; i < totalNodes; i++ {
		var name string
		switch i {
		case 0:
			name = "auth.ValidateSession"
		case 1:
			name = "auth.CheckToken"
		case 2:
			name = "auth.QueryUserDB"
		case 3:
			name = "auth.RefreshToken"
		case 4:
			name = "auth.RevokeSession"
		default:
			name = fmt.Sprintf("pkg%d.Function%d", i/100, i)
		}
		rec := &SymbolRecord{
			ID:      uint16(i),
			Kind:    "func",
			Package: fmt.Sprintf("pkg%d", i/100),
			Name:    name,
		}
		db.symbols = append(db.symbols, rec)
		db.indexByName[name] = rec
	}

	// 2. Conecta grafo determinístico CSR
	// ValidateSession chama CheckToken e RefreshToken
	db.symbols[0].OutgoingCalls = []string{"auth.CheckToken", "auth.RefreshToken"}
	db.symbols[1].IncomingCalls = []string{"auth.ValidateSession"}
	db.symbols[1].OutgoingCalls = []string{"auth.QueryUserDB"}
	db.symbols[2].IncomingCalls = []string{"auth.CheckToken", "auth.RefreshToken"}
	db.symbols[3].IncomingCalls = []string{"auth.ValidateSession"}
	db.symbols[3].OutgoingCalls = []string{"auth.QueryUserDB"}

	db.buildCSR()

	// 3. Constrói índice neural de 8.192 bits (1 KB por nó)
	neuralIdx := NewNeuralIndex(uint32(totalNodes))
	authPattern := make([]uint64, NeuralWordsPerVec)
	for w := 0; w < NeuralWordsPerVec; w++ {
		authPattern[w] = 0xAAAAAAAAAAAAAAAA
	}

	for i := 0; i < totalNodes; i++ {
		vec := make([]uint64, NeuralWordsPerVec)
		if i < 5 {
			copy(vec, authPattern)
			vec[0] ^= uint64(i)
		} else {
			for w := 0; w < NeuralWordsPerVec; w++ {
				vec[w] = rand.Uint64()
			}
		}
		neuralIdx.SetVector(uint32(i), vec)
	}

	b.ResetTimer()
	b.ReportAllocs()

	var totalCandidatesEvaluated int
	for i := 0; i < b.N; i++ {
		res, err := db.QueryNeuralBlastRadius("auth.ValidateSession", neuralIdx, 30, 3)
		if err != nil {
			b.Fatalf("QueryNeuralBlastRadius falhou: %v", err)
		}
		totalCandidatesEvaluated += len(res.SemanticCandidates)
	}
	_ = totalCandidatesEvaluated
}

// TestBlastRadiusEndToEndVerification valida funcionalmente o pipeline completo
func TestBlastRadiusEndToEndVerification(t *testing.T) {
	db := New()
	name0 := "auth.ValidateSession"
	name1 := "auth.CheckToken"
	name2 := "auth.QueryUserDB"

	rec0 := &SymbolRecord{ID: 0, Kind: "func", Name: name0, OutgoingCalls: []string{name1}}
	rec1 := &SymbolRecord{ID: 1, Kind: "func", Name: name1, IncomingCalls: []string{name0}, OutgoingCalls: []string{name2}}
	rec2 := &SymbolRecord{ID: 2, Kind: "func", Name: name2, IncomingCalls: []string{name1}}

	db.symbols = append(db.symbols, rec0, rec1, rec2)
	db.indexByName[name0] = rec0
	db.indexByName[name1] = rec1
	db.indexByName[name2] = rec2

	db.buildCSR()

	neuralIdx := NewNeuralIndex(3)
	vec0 := make([]uint64, NeuralWordsPerVec)
	vec1 := make([]uint64, NeuralWordsPerVec)
	vec2 := make([]uint64, NeuralWordsPerVec)

	// vec0 e vec1 são quase idênticos (apenas 2 bits invertidos)
	vec0[0] = 0xAAAAAAAAAAAAAAAA
	vec1[0] = 0xAAAAAAAAAAAAAAAB
	// vec2 é ortogonal
	vec2[0] = 0x5555555555555555

	neuralIdx.SetVector(0, vec0)
	neuralIdx.SetVector(1, vec1)
	neuralIdx.SetVector(2, vec2)

	res, err := db.QueryNeuralBlastRadius(name0, neuralIdx, 10, 2)
	if err != nil {
		t.Fatalf("Erro na consulta: %v", err)
	}

	if res.TargetSymbol != name0 {
		t.Fatalf("TargetSymbol incorreto: esperado %s, obtido %s", name0, res.TargetSymbol)
	}

	if len(res.SemanticCandidates) == 0 {
		t.Fatalf("Nenhum candidato semântico retornado")
	}
	if res.SemanticCandidates[0].Name != name1 {
		t.Fatalf("Candidato semântico #1 incorreto: esperado %s, obtido %s", name1, res.SemanticCandidates[0].Name)
	}

	if res.TotalAffectedNodes < 2 {
		t.Fatalf("Blast Radius CSR incompleto: esperados pelo menos 2 nós, obtidos %d", res.TotalAffectedNodes)
	}
}
