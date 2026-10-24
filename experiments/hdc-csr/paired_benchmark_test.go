package hdccsr

import (
	"encoding/json"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"
)

type EngineMetrics struct {
	Name            string  `json:"name"`
	RecallAt10      float64 `json:"recall_at_10"`
	PrecisionAt10   float64 `json:"precision_at_10"`
	MRR             float64 `json:"mrr"`
	AvgFalsePos     float64 `json:"avg_false_positives"`
	P50LatencyUs    float64 `json:"p50_latency_us"`
	P95LatencyUs    float64 `json:"p95_latency_us"`
	P99LatencyUs    float64 `json:"p99_latency_us"`
	MeanLatencyUs   float64 `json:"mean_latency_us"`
	QueriesPerSec   float64 `json:"queries_per_sec"`
}

type PairedBenchmarkReport struct {
	TotalQueries int             `json:"total_queries"`
	TotalNodes   int             `json:"total_nodes"`
	K            int             `json:"k"`
	Engines      []EngineMetrics `json:"engines"`
}

// buildBenchmarkEnvironment constrói um ambiente representativo com grafos e semântica correlacionada.
func buildBenchmarkEnvironment(totalNodes int, seed int64) (*ExpCSRGraph, []Hypervector, *ContextualMemory, []QueryRequest, []map[uint16]bool) {
	rng := rand.New(rand.NewSource(seed))

	orig := make([]Hypervector, totalNodes)
	for i := 0; i < totalNodes; i++ {
		orig[i] = GenerateRandomHypervector(rng)
	}

	// Constrói grafo esparso (3 arestas por nó)
	adj := make([][]AdjEdge, totalNodes)
	for i := 0; i < totalNodes; i++ {
		for k := 0; k < 3; k++ {
			target := uint16((i + k + 1) % totalNodes)
			adj[i] = append(adj[i], AdjEdge{Target: target, Rel: RelCall})
		}
	}

	graph := BuildCSRFromAdj(totalNodes, adj)
	hdc := NewHDCSystem(seed)
	hop1 := hdc.PropagateHop1(graph, orig)
	hop2 := hdc.PropagateHop2(graph, hop1)

	// Gera 100 queries de teste com destinos relevantes no 2-hop
	queries := make([]QueryRequest, 0, 100)
	groundTruths := make([]map[uint16]bool, 0, 100)

	for q := 0; q < 100; q++ {
		source := uint16(rng.Intn(totalNodes))
		// Identifica os verdadeiros destinos a 2 saltos
		relevant := make(map[uint16]bool)
		startV := graph.Offsets[source]
		endV := graph.Offsets[source+1]
		for eV := startV; eV < endV; eV++ {
			u := graph.Edges[eV]
			startU := graph.Offsets[u]
			endU := graph.Offsets[u+1]
			for eU := startU; eU < endU; eU++ {
				w := graph.Edges[eU]
				if w != source {
					relevant[w] = true
				}
			}
		}

		if len(relevant) == 0 {
			continue
		}

		queries = append(queries, QueryRequest{
			SourceID: source,
			Semantic: orig[source],
			K:        10,
		})
		groundTruths = append(groundTruths, relevant)
	}

	return graph, orig, hop2, queries, groundTruths
}

func evaluateEngineE2E(engine UnifiedEngine, queries []QueryRequest, groundTruths []map[uint16]bool) EngineMetrics {
	n := len(queries)
	latencies := make([]float64, n)

	totalRecall := 0.0
	totalPrecision := 0.0
	totalMRR := 0.0
	totalFalsePos := 0.0

	for i := 0; i < n; i++ {
		req := queries[i]
		truth := groundTruths[i]

		t0 := time.Now()
		results := engine.QueryTopK(req)
		durUs := float64(time.Since(t0).Nanoseconds()) / 1000.0
		latencies[i] = durUs

		// Avalia acertos
		hits := 0
		firstRankHit := -1
		for r, res := range results {
			if truth[res.NodeID] {
				hits++
				if firstRankHit == -1 {
					firstRankHit = r + 1
				}
			}
		}

		// Recall@K
		if len(truth) > 0 {
			totalRecall += float64(hits) / float64(len(truth))
		}

		// Precision@K (normalizado por min(K, |relevantes|) para neutralizar penalidade estrutural)
		maxPossibleHits := req.K
		if len(truth) < maxPossibleHits {
			maxPossibleHits = len(truth)
		}
		if maxPossibleHits > 0 {
			totalPrecision += float64(hits) / float64(maxPossibleHits)
		}

		// MRR
		if firstRankHit != -1 {
			totalMRR += 1.0 / float64(firstRankHit)
		}

		// Falsos positivos
		totalFalsePos += float64(len(results) - hits)
	}

	sort.Float64s(latencies)
	p50 := latencies[int(float64(n)*0.50)]
	p95 := latencies[int(float64(n)*0.95)]
	p99 := latencies[int(float64(n)*0.99)]
	sumLat := 0.0
	for _, l := range latencies {
		sumLat += l
	}
	mean := sumLat / float64(n)

	qps := 0.0
	if mean > 0 {
		qps = 1000000.0 / mean
	}

	return EngineMetrics{
		Name:          engine.Name(),
		RecallAt10:    totalRecall / float64(n),
		PrecisionAt10: totalPrecision / float64(n),
		MRR:           totalMRR / float64(n),
		AvgFalsePos:   totalFalsePos / float64(n),
		P50LatencyUs:  p50,
		P95LatencyUs:  p95,
		P99LatencyUs:  p99,
		MeanLatencyUs: mean,
		QueriesPerSec: qps,
	}
}

func TestPairedBenchmarkE2E(t *testing.T) {
	const totalNodes = 1000
	graph, orig, hop2, queries, groundTruths := buildBenchmarkEnvironment(totalNodes, 42)

	csrEng := NewCSREngine(graph, orig)
	semEng := NewPureSemanticEngine(totalNodes, orig)
	hdcEng := NewHDCHybridEngine(totalNodes, orig, hop2, 0.5)

	engines := []UnifiedEngine{csrEng, semEng, hdcEng}
	report := PairedBenchmarkReport{
		TotalQueries: len(queries),
		TotalNodes:   totalNodes,
		K:            10,
		Engines:      make([]EngineMetrics, 0, len(engines)),
	}

	t.Logf("=== Benchmark Pareado E2E (%d consultas sobre %d nós) ===", len(queries), totalNodes)
	for _, eng := range engines {
		metrics := evaluateEngineE2E(eng, queries, groundTruths)
		report.Engines = append(report.Engines, metrics)
		t.Logf("[%s]", metrics.Name)
		t.Logf("  Recall@10:    %.2f%%", metrics.RecallAt10*100)
		t.Logf("  Precision@10: %.2f%%", metrics.PrecisionAt10*100)
		t.Logf("  MRR:          %.3f", metrics.MRR)
		t.Logf("  Falsos Pos.:  %.1f", metrics.AvgFalsePos)
		t.Logf("  Latência p50: %.2f µs (p95: %.2f µs, p99: %.2f µs)", metrics.P50LatencyUs, metrics.P95LatencyUs, metrics.P99LatencyUs)
		t.Logf("  Throughput:   %.0f QPS", metrics.QueriesPerSec)
	}

	reportBytes, _ := json.MarshalIndent(report, "", "  ")
	_ = os.WriteFile("paired_metrics.json", reportBytes, 0644)
}

// Benchmarks no formato Go para CI
func BenchmarkE2E_CSR(b *testing.B) {
	graph, orig, _, queries, _ := buildBenchmarkEnvironment(1000, 42)
	eng := NewCSREngine(graph, orig)
	nQ := len(queries)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.QueryTopK(queries[i%nQ])
	}
}

func BenchmarkE2E_Semantic(b *testing.B) {
	_, orig, _, queries, _ := buildBenchmarkEnvironment(1000, 42)
	eng := NewPureSemanticEngine(1000, orig)
	nQ := len(queries)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.QueryTopK(queries[i%nQ])
	}
}

func BenchmarkE2E_HDC(b *testing.B) {
	_, orig, hop2, queries, _ := buildBenchmarkEnvironment(1000, 42)
	eng := NewHDCHybridEngine(1000, orig, hop2, 0.5)
	nQ := len(queries)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = eng.QueryTopK(queries[i%nQ])
	}
}
