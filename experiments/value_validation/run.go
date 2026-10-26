// Package valuevalidation measures bounded engineering utility without assuming
// that either the CSR layout, cache or neural component must win.
package valuevalidation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

type Config struct {
	Roots  []string
	Output string
}

type Pair struct {
	Symbol     string                       `json:"symbol"`
	Fresh      Sample                       `json:"fresh_module_load_and_query"`
	Cached     Sample                       `json:"cache_api_with_source_fingerprint"`
	CacheStats impact.ModuleCacheQueryStats `json:"cache_stats"`
}

type Witness struct {
	Symbol            string   `json:"symbol"`
	StaticDirect      []string `json:"static_direct_callers"`
	CompilerConfirmed []string `json:"compiler_confirmed_callers"`
	Diagnostics       int      `json:"diagnostics"`
	SourceUnmodified  bool     `json:"source_unmodified"`
	Matches           bool     `json:"matches"`
	Error             string   `json:"error,omitempty"`
}

type Case struct {
	Root                string       `json:"root"`
	Error               string       `json:"error,omitempty"`
	SourceSHA256        string       `json:"source_sha256"`
	Packages            int          `json:"packages"`
	Symbols             int          `json:"symbols"`
	Edges               int          `json:"typed_call_sites"`
	AllSymbolsChecked   int          `json:"all_symbols_compared_with_adjacency"`
	FirstLoad           Sample       `json:"first_observed_module_load"`
	CacheInitialization Sample       `json:"graph_cache_initialization"`
	QueryIDs            []string     `json:"query_ids"`
	Pairs               []Pair       `json:"paired_samples"`
	Fresh               Distribution `json:"fresh_total_ms"`
	Cached              Distribution `json:"cached_total_ms"`
	FreshOverCache      float64      `json:"fresh_over_cache_median_ratio"`
	CacheHits           int          `json:"cache_hits"`
	Micro               Micro        `json:"preindexed_query_comparison"`
	FullGraphJSONBytes  int          `json:"full_typed_inventory_json_bytes"`
	AnswerJSONBytes     []int        `json:"answer_json_bytes"`
	Witnesses           []Witness    `json:"compiler_witnesses"`
}

type Report struct {
	Schema     string   `json:"schema"`
	GoVersion  string   `json:"go_version"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	GOMAXPROCS int      `json:"gomaxprocs"`
	Cases      []Case   `json:"cases"`
	Limits     []string `json:"limits"`
}

func Run(ctx context.Context, cfg Config) (Report, error) {
	r := Report{Schema: "mbvm-engineering-value-v1", GoVersion: runtime.Version(), GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, GOMAXPROCS: runtime.GOMAXPROCS(0), Limits: []string{
		"Development measurements on one machine. Failures and bypasses are retained; no positive outcome is required.",
		"Graph parity uses identical typed facts and proves traversal/output equivalence, not full Go runtime call coverage.",
		"Fresh versus cached totals measure index reuse, which conventional tools can also implement. No market superiority claim.",
		"Micro comparison uses a conventional indexed adjacency map with O(1) target lookup and fresh BFS scratch/output.",
		"Compiler export caches are warm in paired trials; first-ever toolchain/disk cold starts are not measured.",
		"Parent allocation counters exclude go-list child-process memory. They are not peak RSS.",
		"JSON bytes measure selected impact context, not exact tokens or lossless source compression.",
		"Compiler witnesses are bounded required-int-parameter mutations, not runtime failure or a formal proof.",
		"Graph cache benefits require the same long-running process; one-shot CLI invocations remain fresh.",
		"No neural benefit is inferred. The previous closed-loop negative result remains unchanged.",
	}}
	if cfg.Output == "" || len(cfg.Roots) == 0 {
		return r, fmt.Errorf("output and roots are required")
	}
	if entries, err := os.ReadDir(cfg.Output); err == nil && len(entries) > 0 {
		return r, fmt.Errorf("output must be new or empty")
	} else if err != nil && !os.IsNotExist(err) {
		return r, fmt.Errorf("inspect output: %w", err)
	}
	if err := os.MkdirAll(cfg.Output, 0700); err != nil {
		return r, fmt.Errorf("create report directory: %w", err)
	}
	for _, root := range cfg.Roots {
		if err := ctx.Err(); err != nil {
			return r, fmt.Errorf("value validation: %w", err)
		}
		c := Case{Root: root}
		if abs, err := filepath.Abs(root); err == nil {
			c.Root = abs
		}
		if err := evaluateCase(ctx, &c); err != nil {
			c.Error = err.Error()
		}
		r.Cases = append(r.Cases, c)
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return r, fmt.Errorf("encode value report: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(cfg.Output, "report.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return r, fmt.Errorf("create value report: %w", err)
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return r, fmt.Errorf("write value report: %w", err)
	}
	if err := f.Close(); err != nil {
		return r, fmt.Errorf("close value report: %w", err)
	}
	return r, nil
}

func evaluateCase(ctx context.Context, c *Case) error {
	var g *impact.ModuleGraph
	first, err := measured(func() error { var err error; g, _, err = impact.LoadModuleWithStats(c.Root); return err })
	c.FirstLoad = first
	if err != nil {
		return fmt.Errorf("load selected module: %w", err)
	}
	if len(g.Symbols) == 0 {
		return fmt.Errorf("module contains no function symbols")
	}
	c.Packages = len(g.Packages)
	c.Symbols = len(g.Symbols)
	c.Edges = len(g.Edges)
	c.SourceSHA256, err = moduleDigest(g)
	if err != nil {
		return err
	}
	a, err := newAdjacency(g)
	if err != nil {
		return err
	}
	for _, symbol := range g.Symbols {
		csr, err := g.Impact(symbol)
		if err != nil {
			return err
		}
		reference, err := a.query(symbol)
		if err != nil {
			return err
		}
		if !equalImpact(csr, reference) {
			return fmt.Errorf("CSR/adjacency mismatch for %s", symbol)
		}
		c.AllSymbolsChecked++
	}
	c.QueryIDs = queries(a)
	inventory := struct {
		Symbols []string            `json:"symbols"`
		Edges   []impact.ModuleEdge `json:"edges"`
	}{g.Symbols, g.Edges}
	raw, err := json.Marshal(inventory)
	if err != nil {
		return fmt.Errorf("encode inventory: %w", err)
	}
	c.FullGraphJSONBytes = len(raw)
	for _, id := range c.QueryIDs {
		q, _ := a.query(id)
		raw, err := json.Marshal(q)
		if err != nil {
			return err
		}
		c.AnswerJSONBytes = append(c.AnswerJSONBytes, len(raw))
	}
	cache := impact.NewModuleCache(1)
	c.CacheInitialization, err = measured(func() error { _, _, err := cache.Impact(c.Root, c.QueryIDs[0]); return err })
	if err != nil {
		return fmt.Errorf("initialize graph cache: %w", err)
	}
	var fresh, cached []float64
	for trial := range 7 {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("paired trial: %w", err)
		}
		p := Pair{Symbol: c.QueryIDs[trial%len(c.QueryIDs)]}
		want, _ := a.query(p.Symbol)
		one := func(useCache bool) error {
			var got impact.ModuleImpact
			s, err := measured(func() error {
				if useCache {
					var err error
					got, p.CacheStats, err = cache.Impact(c.Root, p.Symbol)
					return err
				}
				freshGraph, _, err := impact.LoadModuleWithStats(c.Root)
				if err != nil {
					return err
				}
				got, err = freshGraph.Impact(p.Symbol)
				return err
			})
			if err != nil {
				return err
			}
			if !equalImpact(got, want) {
				return fmt.Errorf("paired API result mismatch")
			}
			if useCache {
				p.Cached = s
			} else {
				p.Fresh = s
			}
			return nil
		}
		if err := one(trial%2 == 0); err != nil {
			return err
		}
		if err := one(trial%2 != 0); err != nil {
			return err
		}
		if p.CacheStats.CacheHit {
			c.CacheHits++
		}
		fresh = append(fresh, p.Fresh.Milliseconds)
		cached = append(cached, p.Cached.Milliseconds)
		c.Pairs = append(c.Pairs, p)
	}
	c.Fresh = distribution(fresh)
	c.Cached = distribution(cached)
	if c.Cached.Median > 0 {
		c.FreshOverCache = c.Fresh.Median / c.Cached.Median
	}
	c.Micro, err = micro(g, a, c.QueryIDs)
	if err != nil {
		return err
	}
	c.Witnesses = compilerWitnesses(g)
	after, err := moduleDigest(g)
	if err != nil {
		return err
	}
	if after != c.SourceSHA256 {
		return fmt.Errorf("source inputs changed during study; discard timing conclusions")
	}
	return nil
}

func compilerWitnesses(g *impact.ModuleGraph) []Witness {
	out := []Witness{}
	for _, symbol := range ordered(g.Symbols) {
		pkg, id, err := g.FindSymbol(symbol)
		if err != nil || strings.Contains(pkg.Symbols[id].Signature, "...") {
			continue
		}
		local, err := pkg.Reachable(id)
		if err != nil || len(local.Direct) == 0 {
			continue
		}
		w := Witness{Symbol: symbol}
		for _, caller := range local.Direct {
			w.StaticDirect = append(w.StaticDirect, pkg.Symbols[caller].Qualified())
		}
		before, err := pkg.SourceSHA256()
		if err != nil {
			w.Error = err.Error()
		} else {
			simulation, err := pkg.SimulateAddIntParameter(id)
			if err != nil {
				w.Error = err.Error()
			} else {
				w.CompilerConfirmed = slices.Clone(simulation.BrokenDirectCallers)
				slices.Sort(w.CompilerConfirmed)
				slices.Sort(w.StaticDirect)
				w.Diagnostics = simulation.TypeDiagnostics
				w.Matches = !simulation.MutatedPackageTypechecked && slices.Equal(w.StaticDirect, w.CompilerConfirmed)
			}
			after, err := pkg.SourceSHA256()
			w.SourceUnmodified = err == nil && before == after
		}
		out = append(out, w)
		if len(out) == 3 {
			break
		}
	}
	return out
}
