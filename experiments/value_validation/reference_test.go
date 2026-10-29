package valuevalidation

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kbelludoo/mbvm-db/internal/impact"
)

func graphFixture() *impact.ModuleGraph {
	return &impact.ModuleGraph{
		Symbols: []string{"a", "b", "c", "d", "isolated"},
		Edges:   []impact.ModuleEdge{{Caller: "a", Callee: "b"}, {Caller: "c", Callee: "b"}, {Caller: "c", Callee: "b"}, {Caller: "d", Callee: "a"}, {Caller: "b", Callee: "d"}},
		Offsets: []uint32{0, 1, 4, 4, 5, 5}, Callers: []uint32{3, 0, 2, 2, 1},
	}
}

func TestReferenceUsesIndependentAdjacencyAndHandlesCycles(t *testing.T) {
	g := graphFixture()
	a, err := newAdjacency(g)
	if err != nil {
		t.Fatal(err)
	}
	b, err := a.query("b")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.DirectCallers, []string{"a", "c"}) || !slices.Equal(b.TransitiveCandidates, []string{"d"}) || b.MaxDepth != 2 {
		t.Fatalf("wrong known graph answer %+v", b)
	}
	for _, id := range g.Symbols {
		csr, err := g.Impact(id)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := a.query(id)
		if err != nil {
			t.Fatal(err)
		}
		if !equalImpact(csr, ref) {
			t.Fatalf("graph parity failed for %s", id)
		}
	}
	g.Callers[0] = 4 // Corrupt ONLY the CSR. The reference must not follow it.
	csr, _ := g.Impact("b")
	ref, _ := a.query("b")
	if equalImpact(csr, ref) || !slices.Equal(ref.TransitiveCandidates, []string{"d"}) {
		t.Fatal("reference accidentally reused CSR representation")
	}
	if _, err := a.query("missing"); err == nil {
		t.Fatal("unknown symbol accepted")
	}
}

func TestDistributionRetainsSamplesAndReportsKnownQuantiles(t *testing.T) {
	x := []float64{7, 2, 1, 6, 3, 5, 4}
	d := distribution(x)
	if d.Median != 4 || d.P95 != 7 || !slices.Equal(x, d.Samples) || x[0] != 7 {
		t.Fatal("wrong quantiles or mutated raw measurements")
	}
	if d := distribution([]float64{1, 3}); d.Median != 2 || d.P95 != 3 {
		t.Fatal("even-sample median failed")
	}
}

func TestValueStudyPreservesFailedCasesAndCompilerEvidence(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	root := t.TempDir()
	files := map[string]string{
		"go.mod":           "module example.test/value\n\ngo 1.27\n",
		"api/api.go":       "package api\nfunc Run(x int)int{return x}\nfunc Wrap(x int)int{return Run(x)}\nfunc Isolated(){}\n",
		"client/client.go": "package client\nimport \"example.test/value/api\"\nfunc Use()int{return api.Wrap(1)}\n",
	}
	for name, source := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(t.TempDir(), "results")
	r, err := Run(t.Context(), Config{Roots: []string{root, filepath.Join(root, "missing")}, Output: out})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Cases) != 2 || r.Cases[0].Error != "" || r.Cases[1].Error == "" {
		t.Fatalf("failed case was dropped or success failed: %+v", r.Cases)
	}
	c := r.Cases[0]
	if c.AllSymbolsChecked != 4 || c.CacheHits != 7 || len(c.Pairs) != 7 {
		t.Fatalf("incomplete measured/parity workload %+v", c)
	}
	if len(c.Witnesses) == 0 || !c.Witnesses[0].Matches || !c.Witnesses[0].SourceUnmodified {
		t.Fatalf("no real compiler witness %+v", c.Witnesses)
	}
	if len(c.Micro.CSR.Samples) != 5 || len(c.Micro.Adjacency.Samples) != 5 {
		t.Fatal("missing indexed micro samples")
	}
	// No assertion about who is faster: a negative measurement is valid evidence.
	if _, err := Run(t.Context(), Config{Roots: []string{root}, Output: out}); err == nil {
		t.Fatal("existing evidence was overwritten")
	}
}
