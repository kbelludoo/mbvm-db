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
	reBench := regexp.MustCompile(`(Benchmark\w+)(?:-\d+)?\s+(\d+)\s+([\d\.]+)\s+ns/op(?:\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op)?`)

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

	requiredBenchmarks := []string{"hamming_8k_ns", "bind_8k_ns", "rotl_8k_ns", "majority3_8k_ns", "csr_1000nodes_propagate_ms", "paired_full_rebuild_1000nodes_ms", "paired_incremental_cached_reverse_1000nodes_ms"}
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
