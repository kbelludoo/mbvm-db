package closedloop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func fixture(t *testing.T) string {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOSUMDB", "off")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.local/closedloop\n\ngo 1.27\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := range 8 {
		source := fmt.Sprintf("package fixture\nfunc HandleItem%d(x int) int { return ComputeItem%d(x) }\nfunc ComputeItem%d(x int) int { return StoreItem%d(x) }\nfunc StoreItem%d(x int) int { return x+%d }\n", i, i, i, i, i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("part%d.go", i)), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func loadFixture(t *testing.T) (*Corpus, *TypedVerifier) {
	t.Helper()
	c, err := LoadCorpus(t.Context(), fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewTypedVerifier(t.Context(), c.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return c, v
}

func TestClosedLoopRealTypedSourcePersistsAndReloads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Directory = fixture(t)
	cfg.Output = filepath.Join(t.TempDir(), "run")
	cfg.Seeds = []int64{42}
	cfg.TrainSteps = 300
	cfg.UpdateSteps = 100
	r, err := Run(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.SeedReports) != 1 {
		t.Fatal("missing seed result")
	}
	seed := r.SeedReports[0]
	if seed.Proposals == 0 || seed.Confirmed+seed.Rejected != seed.Proposals || !seed.HoldoutIsolated {
		t.Fatalf("incomplete closed loop: %+v", seed)
	}
	dir := filepath.Join(cfg.Output, "seed-42")
	initial, h1, err := readState(filepath.Join(dir, "state-0001.mbvmcl"))
	if err != nil {
		t.Fatal(err)
	}
	verified, h2, err := readState(filepath.Join(dir, "verified-before-training.mbvmcl"))
	if err != nil {
		t.Fatal(err)
	}
	final, h3, err := readState(filepath.Join(dir, "state-0003.mbvmcl"))
	if err != nil {
		t.Fatal(err)
	}
	if verified.ParentSHA256 != h1 || final.ParentSHA256 != h2 || seed.VersionOneSHA256 != h1 || seed.VersionTwoSHA256 != h3 {
		t.Fatal("broken provenance chain")
	}
	if initial.ModelVersion != 1 || verified.ModelVersion != 1 || final.ModelVersion != 2 || final.Model.Steps != 400 || verified.Model != initial.Model {
		t.Fatal("model updated before evidence persistence or incorrect version")
	}
	if len(final.Facts) != len(initial.Facts)+seed.Confirmed {
		t.Fatal("unconfirmed proposal became a fact, or confirmed fact disappeared")
	}
	for _, o := range final.Observations {
		if slices.Contains(seed.Split.Test, o.Relation.Caller) || slices.Contains(seed.Split.Test, o.Relation.Callee) {
			t.Fatal("holdout entered acquisition/training")
		}
	}
	for name, m := range seed.Metrics {
		if m.EligibleQueries == 0 || m.Recall < 0 || m.Recall > 1 || m.MRR < 0 || m.MRR > 1 || m.MAP < 0 || m.MAP > 1 {
			t.Fatalf("invalid metric for %s", name)
		}
	}
	if m := seed.Metrics["full_source_typed_oracle_reference"]; m.MRR != 1 || m.MAP != 1 {
		t.Fatalf("exact source reference failed: %+v", m)
	}
	if _, err := Run(t.Context(), cfg); err == nil {
		t.Fatal("overwrote a completed experiment")
	}
}

func TestClosedLoopHoldoutLabelMutationCannotChangeTraining(t *testing.T) {
	c, v := loadFixture(t)
	cfg := DefaultConfig()
	cfg.Output = t.TempDir()
	cfg.TrainSteps = 200
	cfg.UpdateSteps = 100
	a, err := runSeed(t.Context(), c, v, cfg, 42)
	if err != nil {
		t.Fatal(err)
	}
	p := a.Split
	for edge := range c.edges {
		if slices.Contains(p.Test, edge.Caller) || slices.Contains(p.Test, edge.Callee) {
			delete(c.edges, edge)
		}
	}
	// Reverse every held-out caller's labels, including formerly absent edges.
	for _, caller := range p.Test {
		for _, candidate := range c.Symbols {
			c.edges[Relation{caller, candidate.ID}] = true
		}
	}
	cfg.Output = t.TempDir()
	b, err := runSeed(t.Context(), c, v, cfg, 42)
	if err != nil {
		t.Fatal(err)
	}
	if a.VersionOneSHA256 != b.VersionOneSHA256 || a.VersionTwoSHA256 != b.VersionTwoSHA256 || a.Confirmed != b.Confirmed {
		t.Fatal("held-out labels contaminated weights, proposals or persisted facts")
	}
	if a.Metrics["full_source_typed_oracle_reference"].MAP != 1 || b.Metrics["full_source_typed_oracle_reference"].Recall >= a.Metrics["full_source_typed_oracle_reference"].Recall {
		t.Fatal("label mutation did not exercise the evaluator")
	}
}

func TestClosedLoopFileSplitIsDeterministicAndDisjoint(t *testing.T) {
	c, _ := loadFixture(t)
	a, err := c.Partition(42)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Partition(42)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(a.Train, b.Train) || !slices.Equal(a.Acquire, b.Acquire) || !slices.Equal(a.Test, b.Test) {
		t.Fatal("split is not reproducible")
	}
	for _, file := range a.TrainFiles {
		if slices.Contains(a.AcquireFiles, file) || slices.Contains(a.TestFiles, file) {
			t.Fatal("source file crossed split boundary")
		}
	}
	for _, row := range c.examples(a.Train, a.Train) {
		if slices.Contains(a.Test, row.relation.Caller) || slices.Contains(a.Test, row.relation.Callee) || slices.Contains(a.Acquire, row.relation.Callee) {
			t.Fatal("hidden symbol entered bootstrap training")
		}
	}
}

func TestClosedLoopBodyCallNamesDoNotBecomeFeatures(t *testing.T) {
	dir := fixture(t)
	a, err := LoadCorpus(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "part0.go")
	source := "package fixture\nfunc HandleItem0(x int) int { return StoreItem0(x) }\nfunc ComputeItem0(x int) int { return StoreItem0(x) }\nfunc StoreItem0(x int) int { return x }\n"
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := LoadCorpus(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if a.Snapshot.SourceSHA256 == b.Snapshot.SourceSHA256 || len(a.edges) != len(b.edges) {
		t.Fatal("body mutation did not preserve labels count and change provenance")
	}
	for i, n := range a.Symbols {
		if n.ID != b.Symbols[i].ID || n.vector != b.Symbols[i].vector {
			t.Fatal("function body leaked into a symbol representation")
		}
	}
	for i, n := range a.Symbols {
		for j, m := range a.Symbols {
			if features(n, m) != features(b.Symbols[i], b.Symbols[j]) {
				t.Fatal("body calls leaked into pair features")
			}
		}
	}
}

func TestClosedLoopVerifierRejectsStaleSourcesAndContext(t *testing.T) {
	c, v := loadFixture(t)
	path := filepath.Join(c.Snapshot.Directory, "part0.go")
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("\n// changed source bytes\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := v.EnsureUnchanged(); err == nil {
		t.Fatal("same-mtime source change accepted")
	}
	for r := range c.edges {
		if _, err := v.Verify(t.Context(), r, 1, 0.99); err == nil {
			t.Fatal("public verifier accepted a proposal against stale source")
		}
		break
	}
	c, v = loadFixture(t)
	mod := filepath.Join(c.Snapshot.Directory, "go.mod")
	raw, err := os.ReadFile(mod)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mod, append(raw, []byte("\n// dependency context changed\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := v.EnsureUnchanged(); err == nil {
		t.Fatal("changed module context accepted")
	}
}

func TestClosedLoopCanceledRunDoesNotCreateArtifacts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Directory = fixture(t)
	cfg.Output = filepath.Join(t.TempDir(), "must-not-exist")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Run(ctx, cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %s", err)
	}
	if _, err := os.Stat(cfg.Output); !os.IsNotExist(err) {
		t.Fatal("canceled run created output")
	}
}
