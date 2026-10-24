package mbvmdb

import (
	"encoding/json"
	"fmt"
	"math/bits"
	"os"
	"testing"
)

// SemanticSensitivityTestCase estrutura os casos do JSON do Zyon
type SemanticSensitivityTestCase struct {
	ID               string `json:"id"`
	Original         string `json:"original"`
	Equivalent       string `json:"equivalent"`
	Changed          string `json:"changed"`
	Transformation   string `json:"transformation"`
	FunctionalChange string `json:"functional_change"`
}

type SemanticSensitivityDataset struct {
	Schema      string                        `json:"schema"`
	Description string                        `json:"description"`
	Cases       []SemanticSensitivityTestCase `json:"cases"`
}

// TestSemanticSensitivityGoNative implementa em Go puro a avaliação de Sensibilidade Semântica:
// Compara dH(original, equivalente) vs dH(original, alterado) em Go nativo.
func TestSemanticSensitivityGoNative(t *testing.T) {
	casesPath := "../../experiments/hdc-csr/semantic_sensitivity_cases.json"
	data, err := os.ReadFile(casesPath)
	if err != nil {
		t.Skipf("Arquivo de casos não encontrado em %s: %v", casesPath, err)
		return
	}

	var ds SemanticSensitivityDataset
	if err := json.Unmarshal(data, &ds); err != nil {
		t.Fatalf("Erro ao deserializar casos de teste: %v", err)
	}

	t.Logf("==========================================================================================")
	t.Logf("🔬 AVALIAÇÃO DE SENSIBILIDADE SEMÂNTICA 100%% EM GO PURO (SEM RUNTIME PYTHON)")
	t.Logf("   Total de Casos Funcionais do Zyon: %d", len(ds.Cases))
	t.Logf("==========================================================================================")
	t.Logf("%-12s | %-25s | %-25s | %-12s", "ID", "dH(Orig, Equiv)", "dH(Orig, Alterado)", "Status")
	t.Logf("------------------------------------------------------------------------------------------")

	// Projeção baseada em n-gramas e operadores em Go puro
	encodeAST := func(code string) HyperVector8K {
		var vec HyperVector8K
		tokens := tokenizeGoSource(code)
		// 1. Unigramas de operadores e tokens
		for _, tok := range tokens {
			h := fnv1a64(tok)
			vec[int((h>>7)%128)] ^= (1 << (h % 64))
		}
		// 2. Bigramas posicionais para capturar contexto sintático
		for i := 0; i < len(tokens)-1; i++ {
			bigram := tokens[i] + "::" + tokens[i+1]
			h := fnv1a64(bigram)
			vec[int((h>>13)%128)] ^= (1 << (h % 64))
		}
		return vec
	}

	var totalEquivDist float64
	var totalChangedDist float64
	validCases := 0

	for _, c := range ds.Cases {
		vecOrig := encodeAST(c.Original)
		vecEquiv := encodeAST(c.Equivalent)
		vecChanged := encodeAST(c.Changed)

		distEquiv := countHammingDistance(vecOrig, vecEquiv)
		distChanged := countHammingDistance(vecOrig, vecChanged)

		totalEquivDist += float64(distEquiv)
		totalChangedDist += float64(distChanged)
		validCases++

		isSensitive := distChanged > distEquiv
		verdict := "DETECTADA ✅"
		if !isSensitive {
			verdict = "INDETERMINADA ⚠️"
		}

		t.Logf("%-12s | %-25s | %-25s | %-12s",
			c.ID,
			fmt.Sprintf("%d bits (%.2f%%)", distEquiv, float64(distEquiv)*100.0/8192.0),
			fmt.Sprintf("%d bits (%.2f%%)", distChanged, float64(distChanged)*100.0/8192.0),
			verdict,
		)
	}

	avgEquiv := totalEquivDist / float64(validCases)
	avgChanged := totalChangedDist / float64(validCases)

	t.Logf("==========================================================================================")
	t.Logf("📊 MÉDIA GERAL MEDIDA EM GO NATIVO:")
	t.Logf("   Distância Média dH(Original, Equivalente): %.1f bits", avgEquiv)
	t.Logf("   Distância Média dH(Original, Alterado):    %.1f bits", avgChanged)
	t.Logf("==========================================================================================")
}

func countHammingDistance(a, b HyperVector8K) int {
	var dist int
	for i := 0; i < 128; i++ {
		dist += bits.OnesCount64(a[i] ^ b[i])
	}
	return dist
}

func tokenizeGoSource(src string) []string {
	var tokens []string
	var curr []rune
	for _, r := range src {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			curr = append(curr, r)
		} else {
			if len(curr) > 0 {
				tokens = append(tokens, string(curr))
				curr = nil
			}
			if r == '+' || r == '-' || r == '*' || r == '/' || r == '>' || r == '<' || r == '=' || r == '!' || r == '(' || r == ')' {
				tokens = append(tokens, string(r))
			}
		}
	}
	if len(curr) > 0 {
		tokens = append(tokens, string(curr))
	}
	return tokens
}

func fnv1a64(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}
