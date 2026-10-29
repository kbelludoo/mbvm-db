package valuevalidation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

type Sample struct {
	Milliseconds   float64 `json:"milliseconds"`
	AllocatedBytes uint64  `json:"parent_allocated_bytes"`
	Allocations    uint64  `json:"parent_allocations"`
}

type Distribution struct {
	Samples []float64 `json:"milliseconds"`
	Median  float64   `json:"median_ms"`
	P95     float64   `json:"p95_ms"`
}

func distribution(xs []float64) Distribution {
	x := slices.Clone(xs)
	slices.Sort(x)
	d := Distribution{Samples: xs}
	if len(x) > 0 {
		d.Median = x[len(x)/2]
		if len(x)%2 == 0 {
			d.Median = (x[len(x)/2-1] + x[len(x)/2]) / 2
		}
		d.P95 = x[(95*len(x)+99)/100-1]
	}
	return d
}

func measured(f func() error) (Sample, error) {
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	err := f()
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	return Sample{float64(elapsed.Nanoseconds()) / 1e6, after.TotalAlloc - before.TotalAlloc, after.Mallocs - before.Mallocs}, err
}

func moduleDigest(g *impact.ModuleGraph) (string, error) {
	parts := []string{}
	for _, p := range g.Packages {
		hash, err := p.SourceSHA256()
		if err != nil {
			return "", fmt.Errorf("fingerprint package: %w", err)
		}
		parts = append(parts, p.PackagePath+":"+hash)
	}
	slices.Sort(parts)
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		raw, err := os.ReadFile(filepath.Join(g.Root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("fingerprint module metadata: %w", err)
		}
		h.Write([]byte(name))
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func ordered(symbols []string) []string {
	out := slices.Clone(symbols)
	key := func(s string) string { h := sha256.Sum256([]byte("20261026:" + s)); return hex.EncodeToString(h[:]) }
	slices.SortFunc(out, func(a, b string) int { return strings.Compare(key(a), key(b)) })
	return out
}

func queries(a *adjacency) []string {
	var with, without []string
	for _, symbol := range ordered(a.graph.Symbols) {
		q, _ := a.query(symbol)
		if len(q.DirectCallers) > 0 {
			if len(with) < 6 {
				with = append(with, symbol)
			}
		} else if len(without) < 6 {
			without = append(without, symbol)
		}
	}
	return append(with, without...)
}

type Micro struct {
	IterationsPerBatch int          `json:"iterations_per_batch"`
	TimingResolved     bool         `json:"timing_resolved"`
	CSR                Distribution `json:"csr_api_ms_per_query"`
	Adjacency          Distribution `json:"indexed_adjacency_ms_per_query"`
	CSRBytes           float64      `json:"csr_parent_bytes_per_query"`
	AdjacencyBytes     float64      `json:"adjacency_parent_bytes_per_query"`
	CSRAllocs          float64      `json:"csr_parent_allocations_per_query"`
	AdjacencyAllocs    float64      `json:"adjacency_parent_allocations_per_query"`
	AdjacencyOverCSR   float64      `json:"adjacency_over_csr_median_ratio"`
}

var resultSink impact.ModuleImpact

func micro(g *impact.ModuleGraph, a *adjacency, qs []string) (Micro, error) {
	var out Micro
	var csr, adj []float64
	const iterations = 100000
	out.IterationsPerBatch = iterations
	for batch := range 5 {
		one := func(useCSR bool) error {
			runtime.GC()
			s, err := measured(func() error {
				for i := range iterations {
					var r impact.ModuleImpact
					var err error
					if useCSR {
						r, err = g.Impact(qs[i%len(qs)])
					} else {
						r, err = a.query(qs[i%len(qs)])
					}
					if err != nil {
						return err
					}
					resultSink = r
				}
				return nil
			})
			if err != nil {
				return err
			}
			if useCSR {
				csr = append(csr, s.Milliseconds/iterations)
				out.CSRBytes += float64(s.AllocatedBytes) / iterations / 5
				out.CSRAllocs += float64(s.Allocations) / iterations / 5
			} else {
				adj = append(adj, s.Milliseconds/iterations)
				out.AdjacencyBytes += float64(s.AllocatedBytes) / iterations / 5
				out.AdjacencyAllocs += float64(s.Allocations) / iterations / 5
			}
			return nil
		}
		if err := one(batch%2 == 0); err != nil {
			return out, err
		}
		if err := one(batch%2 != 0); err != nil {
			return out, err
		}
	}
	out.CSR = distribution(csr)
	out.Adjacency = distribution(adj)
	out.TimingResolved = out.CSR.Median > 0 && out.Adjacency.Median > 0
	if out.TimingResolved {
		out.AdjacencyOverCSR = out.Adjacency.Median / out.CSR.Median
	}
	return out, nil
}
