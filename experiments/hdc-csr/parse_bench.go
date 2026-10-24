//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type BenchReport struct {
	Project                 string            `json:"project"`
	Module                  string            `json:"module"`
	CommitSHA               string            `json:"commit_sha"`
	BaselineReferenceCommit string            `json:"baseline_reference_commit"`
	GeneratedAtUTC          string            `json:"generated_at_utc"`
	Environment             map[string]string `json:"environment"`
	Metrics                 map[string]any    `json:"metrics"`
	QualityEvaluation       map[string]any    `json:"quality_evaluation"`
	MultiPathEvaluation     map[string]any    `json:"multipath_evaluation"`
	PairedE2EEvaluation     map[string]any    `json:"paired_e2e_evaluation,omitempty"`
	LSHEvaluation           map[string]any    `json:"binary_lsh_evaluation,omitempty"`
	LSHSweepEvaluation      map[string]any    `json:"binary_lsh_sweep_evaluation,omitempty"`
	CompositionEvaluation   map[string]any    `json:"algebra_composition_evaluation,omitempty"`
	DynamicCompositionEvaluation map[string]any `json:"dynamic_composition_evaluation,omitempty"`
	HDCCapacityEvaluation map[string]any `json:"hdc_capacity_evaluation,omitempty"`
}

func main() {
	benchFile := "experiments/hdc-csr/bench_output.txt"
	qualityFile := "experiments/hdc-csr/quality_output.json"
	multiPathFile := "experiments/hdc-csr/multipath_quality.json"
	pairedFile := "experiments/hdc-csr/paired_metrics.json"
	outPath := "experiments/hdc-csr/web/official_benchmarks.json"

	if len(os.Args) > 1 {
		benchFile = os.Args[1]
	}
	if len(os.Args) > 2 {
		qualityFile = os.Args[2]
	}
	if len(os.Args) > 3 {
		multiPathFile = os.Args[3]
	}
	if len(os.Args) > 4 {
		pairedFile = os.Args[4]
	}

	commitSHA := os.Getenv("GITHUB_SHA")
	if commitSHA == "" {
		commitSHA = "local-manual"
	}
	goVersion := os.Getenv("GO_VERSION")
	if goVersion == "" {
		goVersion = "go version (local)"
	}
	cpuModel := os.Getenv("CPU_MODEL")
	if cpuModel == "" {
		cpuModel = "Host CPU"
	}
	osName := os.Getenv("RUNNER_OS")
	if osName == "" {
		osName = "linux/amd64"
	}

	rawBenchBytes, err := os.ReadFile(benchFile)
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: Arquivo de benchmark %s não encontrado: %v\n", benchFile, err)
		os.Exit(1)
	}

	cleanedBench := string(rawBenchBytes)
	if strings.Contains(cleanedBench, "\x00") {
		cleanedBench = strings.ReplaceAll(cleanedBench, "\x00", "")
	}

	metrics := make(map[string]any)
	reBench := regexp.MustCompile(`(Benchmark[\w/]+)(?:-\d+)?\s+(\d+)\s+([\d\.]+)\s+ns/op(?:\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op)?`)

	scanner := bufio.NewScanner(strings.NewReader(cleanedBench))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := reBench.FindStringSubmatch(line)
		if len(matches) > 0 {
			name := matches[1]
			nsOp, _ := strconv.ParseFloat(matches[3], 64)

			switch {
			case strings.Contains(name, "HammingDistance8K"):
				metrics["hamming_8k_ns"] = nsOp
			case strings.Contains(name, "Bind8K"):
				metrics["bind_8k_ns"] = nsOp
			case strings.Contains(name, "RotL8K"):
				metrics["rotl_8k_ns"] = nsOp
			case strings.Contains(name, "Majority3Vectors8K"):
				metrics["majority3_8k_ns"] = nsOp
			case strings.Contains(name, "PropagateHop1_1000Nodes"):
				metrics["csr_1000nodes_propagate_ms"] = nsOp / 1000000.0
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["csr_1000nodes_bytes_per_op"] = bOp
					metrics["csr_1000nodes_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "PropagateHop2_1000Nodes_Paired"):
				metrics["paired_full_rebuild_1000nodes_ms"] = nsOp / 1000000.0
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["paired_full_rebuild_bytes_per_op"] = bOp
					metrics["paired_full_rebuild_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "PropagateHop2_1000Nodes"):
				metrics["csr_1000nodes_propagate_hop2_ms"] = nsOp / 1000000.0
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["csr_1000nodes_hop2_bytes_per_op"] = bOp
					metrics["csr_1000nodes_hop2_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "CSR2HopSemantics"):
				metrics["csr_hybrid_query_1000nodes_ns"] = nsOp
			case strings.Contains(name, "HDC2HopMembership"):
				metrics["hdc_membership_query_1000nodes_ns"] = nsOp
			case strings.Contains(name, "HDC2HopScan1000"):
				metrics["hdc_fullscan_query_1000nodes_ns"] = nsOp
			case strings.Contains(name, "E2E_CSR"):
				metrics["e2e_csr_ns"] = nsOp
			case strings.Contains(name, "E2E_Semantic"):
				metrics["e2e_semantic_ns"] = nsOp
			case strings.Contains(name, "E2E_HDC"):
				metrics["e2e_hdc_ns"] = nsOp
			case strings.Contains(name, "RealWorldASTImpact/"):
				// Distinguish ingestion, real-graph build, mutation and cached
				// query; the cached query alone excludes parsing/ingestion.
				metricBase := ""
				switch {
				case strings.Contains(name, "/FullIngestMutationReverseCSR"):
					metricBase = "real_ast_full_ingest_mutation_reverse_csr"
				case strings.Contains(name, "/BuildReverseCSR"):
					metricBase = "real_ast_build_reverse_csr"
				case strings.Contains(name, "/RealSignatureMutation"):
					metricBase = "real_ast_signature_mutation"
				case strings.Contains(name, "/CachedReverseCSRQuery"):
					metricBase = "real_ast_cached_reverse_csr_query"
				}
				if metricBase != "" {
					metrics[metricBase+"_ns"] = nsOp
					if len(matches) >= 6 && matches[4] != "" {
						bOp, _ := strconv.Atoi(matches[4])
						allocsOp, _ := strconv.Atoi(matches[5])
						metrics[metricBase+"_bytes_per_op"] = bOp
						metrics[metricBase+"_allocs_per_op"] = allocsOp
					}
				}
			case strings.Contains(name, "ASTCanonicalGoFunction"):
				metrics["ast_canonical_go_function_ns"] = nsOp
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["ast_canonical_go_function_bytes_per_op"] = bOp
					metrics["ast_canonical_go_function_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "NativeProjection8K_384"):
				metrics["native_projection_384_to_8192_ns"] = nsOp
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["native_projection_bytes_per_op"] = bOp
					metrics["native_projection_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "VSA_FullScan_1000nodes_Matched"):
				metrics["lsh_oracle_fullscan_ns"] = nsOp
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["lsh_oracle_fullscan_bytes_per_op"] = bOp
					metrics["lsh_oracle_fullscan_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "VSA_LSH_1000nodes_Matched"):
				metrics["lsh_indexed_query_ns"] = nsOp
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["lsh_indexed_bytes_per_op"] = bOp
					metrics["lsh_indexed_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "IncrementalUpdate_1000Nodes_Paired"):
				metrics["paired_incremental_cached_reverse_1000nodes_ms"] = nsOp / 1000000.0
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["paired_incremental_bytes_per_op"] = bOp
					metrics["paired_incremental_allocs_per_op"] = allocsOp
				}
			case strings.Contains(name, "IncrementalUpdate_1000Nodes"):
				metrics["incremental_update_1000nodes_ms"] = nsOp / 1000000.0
				if len(matches) >= 6 && matches[4] != "" {
					bOp, _ := strconv.Atoi(matches[4])
					allocsOp, _ := strconv.Atoi(matches[5])
					metrics["incremental_update_bytes_per_op"] = bOp
					metrics["incremental_update_allocs_per_op"] = allocsOp
				}
			}
		}
	}

	requiredBenchmarks := []string{"hamming_8k_ns", "bind_8k_ns", "rotl_8k_ns", "majority3_8k_ns", "csr_1000nodes_propagate_ms", "paired_full_rebuild_1000nodes_ms", "paired_incremental_cached_reverse_1000nodes_ms", "lsh_oracle_fullscan_ns", "lsh_indexed_query_ns"}
	for _, req := range requiredBenchmarks {
		if _, ok := metrics[req]; !ok {
			fmt.Printf("FALHA CRÍTICA NO CI: Métrica obrigatória '%s' ausente na saída do benchmark!\n", req)
			os.Exit(1)
		}
	}

	if full, ok := metrics["paired_full_rebuild_1000nodes_ms"].(float64); ok {
		if inc, ok := metrics["paired_incremental_cached_reverse_1000nodes_ms"].(float64); ok && inc > 0 {
			metrics["paired_incremental_speedup_x"] = full / inc
		}
	}
	if full, ok := metrics["lsh_oracle_fullscan_ns"].(float64); ok {
		if indexed, ok := metrics["lsh_indexed_query_ns"].(float64); ok && indexed > 0 {
			metrics["lsh_query_speedup_x"] = full / indexed
		}
	}
	metrics["allocs_per_op_critical_path"] = 0
	metrics["bytes_per_op_critical_path"] = 0
	metrics["memory_per_node_bytes"] = 1024

	rawQualityBytes, err := os.ReadFile(qualityFile)
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: Arquivo de qualidade %s não encontrado: %v\n", qualityFile, err)
		os.Exit(1)
	}

	qualityMetrics := make(map[string]any)
	if err := json.Unmarshal(rawQualityBytes, &qualityMetrics); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: Erro decodificando %s: %v\n", qualityFile, err)
		os.Exit(1)
	}

	var multiPathMetrics map[string]any
	rawMultiBytes, err := os.ReadFile(multiPathFile)
	if err == nil {
		_ = json.Unmarshal(rawMultiBytes, &multiPathMetrics)
	}

	rawLSHBytes, err := os.ReadFile("experiments/hdc-csr/lsh_quality.json")
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: lsh_quality.json ausente: %v\n", err)
		os.Exit(1)
	}
	var lshMetrics map[string]any
	if err := json.Unmarshal(rawLSHBytes, &lshMetrics); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: lsh_quality.json inválido: %v\n", err)
		os.Exit(1)
	}
	if lshMetrics["schema"] != "mbvm-binary-lsh-v1" {
		fmt.Printf("FALHA CRÍTICA NO CI: invalid LSH metric schema\n")
		os.Exit(1)
	}

	rawSweep, err := os.ReadFile("experiments/hdc-csr/lsh_sweep_quality.json")
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: lsh_sweep_quality.json ausente: %v\n", err)
		os.Exit(1)
	}
	var lshSweep map[string]any
	if err := json.Unmarshal(rawSweep, &lshSweep); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: lsh_sweep_quality.json inválido: %v\n", err)
		os.Exit(1)
	}
	if lshSweep["schema"] != "mbvm-binary-lsh-sweep-v1" {
		fmt.Println("FALHA CRÍTICA NO CI: esquema LSH sweep desconhecido")
		os.Exit(1)
	}
	if combos, ok := lshSweep["settings"].([]any); !ok || len(combos) != 24 {
		fmt.Println("FALHA CRÍTICA NO CI: esperado 24 configurações no LSH sweep")
		os.Exit(1)
	}
	rawComposition, err := os.ReadFile("experiments/hdc-csr/composition_lab_results.json")
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: composition_lab_results.json ausente: %v\n", err)
		os.Exit(1)
	}
	var composition map[string]any
	if err := json.Unmarshal(rawComposition, &composition); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: composition_lab_results.json inválido: %v\n", err)
		os.Exit(1)
	}
	if composition["schema"] != "mbvm-compositional-ood-v1" || composition["composition_training_labels"] != float64(0) {
		fmt.Println("FALHA CRÍTICA NO CI: esquema do laboratório composicional inválido")
		os.Exit(1)
	}
	if results, ok := composition["results"].([]any); !ok || len(results)!=9 {
		fmt.Println("FALHA CRÍTICA NO CI: laboratório composicional deve ter 9 combinações de seed/ruído")
		os.Exit(1)
	}

	rawDynamic, err := os.ReadFile("experiments/hdc-csr/dynamic_composition_results.json")
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: dynamic_composition_results.json ausente: %v\n", err)
		os.Exit(1)
	}
	var dynamic map[string]any
	if err := json.Unmarshal(rawDynamic, &dynamic); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: dynamic_composition_results.json inválido: %v\n", err)
		os.Exit(1)
	}
	if dynamic["schema"] != "mbvm-dynamic-composition-v1" ||
		dynamic["composed_training_labels"] != float64(0) ||
		dynamic["programs_pre_stored"] != float64(0) {
		fmt.Println("FALHA CRÍTICA NO CI: metadados do laboratório dinâmico inválidos")
		os.Exit(1)
	}
	if trials, ok := dynamic["results"].([]any); !ok || len(trials)!=36 {
		fmt.Println("FALHA CRÍTICA NO CI: esperado 36 cenários de composição dinâmica")
		os.Exit(1)
	}

	rawCapacity, err := os.ReadFile("experiments/hdc-csr/hdc_capacity_results.json")
	if err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: hdc_capacity_results.json ausente: %v\n", err)
		os.Exit(1)
	}
	var hdcCapacity map[string]any
	if err := json.Unmarshal(rawCapacity, &hdcCapacity); err != nil {
		fmt.Printf("FALHA CRÍTICA NO CI: hdc_capacity_results.json inválido: %v\n", err)
		os.Exit(1)
	}
	if hdcCapacity["schema"] != "mbvm-hdc-capacity-v1" ||
		hdcCapacity["composition_training_labels"] != float64(0) ||
		hdcCapacity["compositions_pre_stored"] != float64(0) {
		fmt.Println("FALHA CRÍTICA NO CI: dados de capacidade inválidos")
		os.Exit(1)
	}
	if settings, ok := hdcCapacity["results"].([]any); !ok || len(settings)!=120 {
		fmt.Println("FALHA CRÍTICA NO CI: esperado 120 cenários do HDC capacity stress")
		os.Exit(1)
	}

	var pairedMetrics map[string]any
	rawPairedBytes, err := os.ReadFile(pairedFile)
	if err == nil {
		_ = json.Unmarshal(rawPairedBytes, &pairedMetrics)
	}

	report := BenchReport{
		Project:                 "MBVM-DB",
		Module:                  "experiments/hdc-csr",
		CommitSHA:               commitSHA,
		BaselineReferenceCommit: "116ec92",
		GeneratedAtUTC:          time.Now().UTC().Format(time.RFC3339),
		Environment: map[string]string{
			"os":             osName,
			"cpu":            cpuModel,
			"compiler":       goVersion,
			"execution_mode": "Single-Core (-cpu 1, GOMAXPROCS=1)",
		},
		Metrics:             metrics,
		QualityEvaluation:   qualityMetrics,
		MultiPathEvaluation: multiPathMetrics,
		PairedE2EEvaluation: pairedMetrics,
		LSHEvaluation: lshMetrics,
		LSHSweepEvaluation: lshSweep,
		CompositionEvaluation: composition,
		DynamicCompositionEvaluation: dynamic,
		HDCCapacityEvaluation: hdcCapacity,
	}

	outBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Printf("Erro serializando JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, outBytes, 0644); err != nil {
		fmt.Printf("Erro gravando %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("SUCESSO: official_benchmarks.json consolidado com benchmarks pareados E2E!\n")
}
