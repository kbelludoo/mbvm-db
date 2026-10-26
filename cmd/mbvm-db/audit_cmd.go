package main

import (
	"fmt"
	"os"

	"github.com/kbelludoo/mbvm-db/pkg/mbvmdb"
)

func runAudit(target string) {
	rep, err := mbvmdb.AuditBotCoverage(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro ao auditar bot: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n🔍 [MBVM-DB] AUDITORIA DE COBERTURA DE REGRAS DO JOGO\n")
	fmt.Printf("   Projeto: %s\n", target)
	fmt.Printf("   ─────────────────────────────────────────────────────────────\n")
	fmt.Printf("   📊 Cobertura Total: %.1f%% (%d de %d mecânicas/regras)\n\n",
		rep.CoveragePercent, rep.CoveredFeatures, rep.TotalGameFeatures)

	fmt.Printf("   ✅ MECÂNICAS IMPLEMENTADAS (%d):\n", len(rep.CoveredList))
	limitCov := 12
	if len(rep.CoveredList) < limitCov {
		limitCov = len(rep.CoveredList)
	}
	for i := 0; i < limitCov; i++ {
		fmt.Printf("      • %s\n", rep.CoveredList[i])
	}
	if len(rep.CoveredList) > limitCov {
		fmt.Printf("      ... e mais %d recursos implementados.\n", len(rep.CoveredList)-limitCov)
	}

	fmt.Printf("\n   ⚠️  MECÂNICAS DO JOGO AUSENTES NO BOT (%d):\n", len(rep.MissingList))
	limitMiss := 15
	if len(rep.MissingList) < limitMiss {
		limitMiss = len(rep.MissingList)
	}
	for i := 0; i < limitMiss; i++ {
		fmt.Printf("      ❌ %s\n", rep.MissingList[i])
	}
	if len(rep.MissingList) > limitMiss {
		fmt.Printf("      ... e mais %d recursos não cobertos.\n", len(rep.MissingList)-limitMiss)
	}
	fmt.Printf("   ─────────────────────────────────────────────────────────────\n\n")
}
