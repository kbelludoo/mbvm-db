package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	realeval "github.com/kbelludoo/mbvm-db/experiments/real_eval"
)

func main() {
	singleFlag := flag.Bool("single", false, "Run single integration test before full benchmark")
	pilotFlag := flag.Bool("pilot", false, "Run 4-problem pilot across 4 conditions (16 trials)")
	unseenFlag := flag.Bool("unseen", false, "Run 20-problem unexposed multi-lemma benchmark (80 trials)")
	fullFlag := flag.Bool("full", false, "Run 30-problem calibration benchmark across 4 conditions (120 trials)")
	modelFlag := flag.String("model", "gemini-2.5-flash", "Gemini model to evaluate")
	keyFlag := flag.String("api-key", "", "Google GenAI / Gemini API key")
	reportFlag := flag.String("report", "experiments/real_eval/live_benchmark_results.json", "Report output JSON path")
	flag.Parse()

	// Default to single if no action specified
	if !*singleFlag && !*pilotFlag && !*unseenFlag && !*fullFlag {
		*singleFlag = true
	}

	if *keyFlag != "" {
		_ = os.Setenv("GEMINI_API_KEY", *keyFlag)
	}

	client, err := realeval.NewGeminiClient(*modelFlag)
	if err != nil {
		fmt.Printf("❌ Erro de inicialização do cliente Gemini:\n   %v\n\n", err)
		fmt.Println("Defina a variável de ambiente GEMINI_API_KEY ou use --api-key=<sua_chave>.")
		os.Exit(1)
	}

	runner := realeval.NewRunner(client, *reportFlag)
	ctx := context.Background()

	if *singleFlag {
		fmt.Printf("🧪 [MBVM-DB] Executando Teste de Integração (Modelo: %s)...\n", *modelFlag)
		result, err := runner.RunSingleIntegrationTest(ctx)
		if err != nil {
			fmt.Printf("❌ Falha no teste de integração: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Teste de Integração Concluído com Sucesso!\n")
		fmt.Printf("   • Problema: %s\n", result.ProblemID)
		fmt.Printf("   • Sucesso: %v\n", result.Success)
		fmt.Printf("   • Status de Verificação: %s\n", result.VerificationStatus)
		fmt.Printf("   • Tokens de Prompt Reais: %d\n", result.PromptTokensReal)
		fmt.Printf("   • Tokens de Resposta Reais: %d\n", result.CompletionTokensReal)
		fmt.Printf("   • Tokens de Raciocínio (Thoughts): %d\n", result.ReasoningTokensReal)
		fmt.Printf("   • Tokens Totais Reais: %d\n", result.TotalTokensReal)
		fmt.Printf("   • Latência: %v\n", result.Latency)
		fmt.Printf("   • Caminho CSR: %s\n", result.RetrievalPath)
		fmt.Printf("   • Resposta do Modelo:\n%s\n", result.ResponseText)
		return
	}

	if *pilotFlag {
		fmt.Printf("🔬 [MBVM-DB] Executando Piloto Científico (4 Problemas x 4 Condições = 16 Ensaios, Modelo: %s)...\n", *modelFlag)
		_, summaries, err := runner.RunPilotBenchmark(ctx)
		if err != nil {
			fmt.Printf("❌ Falha no piloto: %v\n", err)
			os.Exit(1)
		}
		printSummaries(summaries, *reportFlag)
		return
	}

	if *unseenFlag {
		fmt.Printf("🛡️ [MBVM-DB] Executando Benchmark Inédito Oculto (20 Problemas x 4 Condições = 80 Ensaios, Modelo: %s)...\n", *modelFlag)
		_, summaries, err := runner.RunUnseenBenchmark(ctx)
		if err != nil {
			fmt.Printf("❌ Falha no benchmark oculto: %v\n", err)
			os.Exit(1)
		}
		printSummaries(summaries, *reportFlag)
		return
	}

	if *fullFlag {
		fmt.Printf("🚀 [MBVM-DB] Iniciando Benchmark de Calibração (30 Problemas x 4 Condições = 120 Ensaios, Modelo: %s)...\n", *modelFlag)
		_, summaries, err := runner.RunFullBenchmark(ctx)
		if err != nil {
			fmt.Printf("❌ Falha no benchmark: %v\n", err)
			os.Exit(1)
		}
		printSummaries(summaries, *reportFlag)
	}
}

func printSummaries(summaries []realeval.BenchmarkSummary, reportPath string) {
	fmt.Printf("\n✅ Avaliação Concluída! Relatório gravado em: %s\n\n", reportPath)
	fmt.Println("=== Resumo Comparativo Oficial das 4 Condições ===")
	for _, s := range summaries {
		fmt.Printf("[%s]\n", s.Condition)
		fmt.Printf("  • Acurácia: %d/%d (%.1f%%)\n", s.SolvedCount, s.TotalProblems, s.Accuracy*100)
		fmt.Printf("  • Tokens Médios de Prompt: %.1f\n", s.AvgPromptTokens)
		fmt.Printf("  • Tokens Médios de Resposta: %.1f\n", s.AvgCompleteTokens)
		fmt.Printf("  • Tokens Médios Totais: %.1f\n", s.AvgTotalTokens)
		fmt.Printf("  • Latência Média: %.1f ms\n\n", s.AvgLatencyMs)
	}
}
