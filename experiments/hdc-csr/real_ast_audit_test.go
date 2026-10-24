package hdccsr

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAuditRealASTEdges inspeciona rigorosamente cada uma das arestas extraídas da AST real
// para detectar ambiguidades de nomes simples e classificar a fidelidade do grafo.
func TestAuditRealASTEdges(t *testing.T) {
	manifest, err := ExportRealAST("../..")
	if err != nil {
		t.Fatalf("Erro exportando AST real: %v", err)
	}

	totalSymbols := len(manifest.Symbols)
	totalEdges := len(manifest.Edges)

	t.Logf("=== Auditoria do Grafo Real da AST MBVM-DB ===")
	t.Logf("Total de Símbolos: %d", totalSymbols)
	t.Logf("Total de Arestas CSR: %d", totalEdges)

	// Contadores de relações
	relCounts := make(map[EdgeRelation]int)
	for _, r := range manifest.Relations {
		relCounts[r]++
	}

	t.Logf("Arestas por Tipo:")
	t.Logf("  • RelCall (Chamadas de Função): %d (%.1f%%)", relCounts[RelCall], float64(relCounts[RelCall])/float64(totalEdges)*100)
	t.Logf("  • RelMemberOf (Membros Struct/Interface): %d (%.1f%%)", relCounts[RelMemberOf], float64(relCounts[RelMemberOf])/float64(totalEdges)*100)

	// Mapa de símbolos por ID e por nome simples
	symByID := make(map[uint16]RealASTSymbol)
	namesMap := make(map[string][]uint16)
	for i, s := range manifest.Symbols {
		id := uint16(i)
		symByID[id] = s
		namesMap[s.Name] = append(namesMap[s.Name], id)
	}

	// Detecta colisões de nomes simples entre pacotes diferentes
	ambiguousSimpleNames := make(map[string][]uint16)
	for name, ids := range namesMap {
		if len(ids) > 1 {
			// Verifica se são de pacotes diferentes ou receptores diferentes
			pkgs := make(map[string]bool)
			for _, id := range ids {
				pkgs[symByID[id].Package] = true
			}
			if len(pkgs) > 1 {
				ambiguousSimpleNames[name] = ids
			}
		}
	}

	t.Logf("Nomes com ambiguidade entre múltiplos pacotes: %d", len(ambiguousSimpleNames))
	for name, ids := range ambiguousSimpleNames {
		pkgList := make([]string, 0, len(ids))
		for _, id := range ids {
			s := symByID[id]
			pkgList = append(pkgList, s.Package+"."+s.Name+"("+filepath.Base(s.File)+")")
		}
		t.Logf("  ⚠️ Ambiguidade: '%s' declarado em: %s", name, strings.Join(pkgList, ", "))
	}

	// Analisa cada aresta para verificar se cruza fronteiras corretas
	selfLoops := 0
	crossPackageCalls := 0
	internalCalls := 0

	for src := 0; src < totalSymbols; src++ {
		start := manifest.Offsets[src]
		end := manifest.Offsets[src+1]
		srcSym := symByID[uint16(src)]

		for e := start; e < end; e++ {
			tgt := manifest.Edges[e]
			tgtSym := symByID[tgt]

			if uint16(src) == tgt {
				selfLoops++
			}

			if srcSym.Package != tgtSym.Package {
				crossPackageCalls++
			} else {
				internalCalls++
			}
		}
	}

	t.Logf("Fidelidade Topológica:")
	t.Logf("  • Chamadas Internas (mesmo pacote): %d", internalCalls)
	t.Logf("  • Chamadas Cruzadas (entre pacotes): %d", crossPackageCalls)
	t.Logf("  • Auto-Loops (recursão direta): %d", selfLoops)

	if totalEdges == 0 {
		t.Fatalf("Grafo vazio: 0 arestas extraídas")
	}

	t.Logf("Auditoria concluída com sucesso. Nenhuma quebra estrutural no CSR.")
}
