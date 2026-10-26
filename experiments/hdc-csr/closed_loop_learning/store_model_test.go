package closedloop

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func validState(t *testing.T) State {
	t.Helper()
	c, v := loadFixture(t)
	p, err := c.Partition(42)
	if err != nil {
		t.Fatal(err)
	}
	s := State{Schema: "mbvm-closed-loop-state-v1", Version: 1, ModelVersion: 1, Snapshot: c.Snapshot, Split: p, Model: newModel(42)}
	for _, row := range c.examples(p.Train, p.Train) {
		if row.y != 1 {
			continue
		}
		o, err := v.Verify(t.Context(), row.relation, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		s.Observations = append(s.Observations, o)
		s.Facts = append(s.Facts, Fact{o, 1})
	}
	if len(s.Facts) == 0 {
		t.Fatal("fixture has no bootstrap facts")
	}
	return s
}

func TestClosedLoopStateDetectsCorruptionAndDoesNotOverwrite(t *testing.T) {
	s := validState(t)
	path := filepath.Join(t.TempDir(), "state.mbvmcl")
	hash, err := writeState(path, s)
	if err != nil {
		t.Fatal(err)
	}
	copy, loaded, err := readState(path)
	if err != nil || loaded != hash || copy.Model != s.Model {
		t.Fatal("state did not round-trip")
	}
	if _, err := writeState(path, s); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing revision was not protected: %s", err)
	}
	if _, same, err := readState(path); err != nil || same != hash {
		t.Fatal("overwrite attempt changed old revision")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int{0, 8, 15, len(raw) - 1} {
		other := filepath.Join(t.TempDir(), "truncated.mbvmcl")
		if err := os.WriteFile(other, raw[:size], 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readState(other); err == nil {
			t.Fatal("truncated state accepted")
		}
	}
	raw[len(raw)-1] ^= 1
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readState(path); err == nil {
		t.Fatal("CRC corruption accepted")
	}
}

func TestClosedLoopStateCannotPromoteUnverifiedScores(t *testing.T) {
	base := validState(t)
	t.Run("score-only", func(t *testing.T) {
		s := base
		s.Observations = nil
		s.Facts = nil
		o := Observation{Relation: Relation{s.Split.Train[0], s.Split.Train[1]}, Score: 0.999, Confirmed: true}
		s.Observations = append(s.Observations, o)
		s.Facts = append(s.Facts, Fact{o, 1})
		if err := s.validate(); err == nil {
			t.Fatal("score-only evidence accepted")
		}
	})
	t.Run("holdout", func(t *testing.T) {
		s := base
		s.Observations = append([]Observation(nil), base.Observations...)
		s.Observations[0].Relation.Caller = s.Split.Test[0]
		if err := s.validate(); err == nil {
			t.Fatal("holdout observation accepted")
		}
	})
	t.Run("negative-fact", func(t *testing.T) {
		s := base
		s.Observations = append([]Observation(nil), base.Observations...)
		s.Facts = append([]Fact(nil), base.Facts...)
		s.Observations[0].Confirmed = false
		s.Facts[0].Evidence = s.Observations[0]
		if err := s.validate(); err == nil {
			t.Fatal("negative observation became a fact")
		}
	})
	t.Run("duplicate-splits", func(t *testing.T) {
		s := base
		s.Split.Acquire = append([]string{s.Split.Train[0]}, s.Split.Acquire...)
		if err := s.validate(); err == nil {
			t.Fatal("overlapping split groups accepted")
		}
	})
}

func TestClosedLoopMLPLearnsLabelsAndPreservesFrozenControl(t *testing.T) {
	var negative, positive example
	positive.x[0] = 1
	positive.y = 1
	m := newModel(42)
	frozen := m
	oldLoss := math.Pow(m.score(positive.x)-1, 2) + math.Pow(m.score(negative.x), 2)
	if err := m.train(t.Context(), []example{positive, negative}, 2000, 42); err != nil {
		t.Fatal(err)
	}
	newLoss := math.Pow(m.score(positive.x)-1, 2) + math.Pow(m.score(negative.x), 2)
	if newLoss >= oldLoss/10 || m.score(positive.x) <= 0.9 || m.score(negative.x) >= 0.1 {
		t.Fatalf("learner did not fit independently specified labels: old=%f new=%f", oldLoss, newLoss)
	}
	if frozen != newModel(42) || m.Steps != 2000 {
		t.Fatal("frozen control mutated or budget not respected")
	}
	m.Input[0][0] = math.NaN()
	if err := m.validate(); err == nil {
		t.Fatal("nonfinite model accepted")
	}
}

func TestClosedLoopIndependentVerifierUsesTypedReceiverIdentity(t *testing.T) {
	dir := fixture(t)
	src := "package fixture\ntype A struct{}\ntype B struct{}\nfunc (A) Apply() {}\nfunc (B) Apply() {}\nfunc CallA(a A) { a.Apply() }\ntype Caller interface{ Apply() }\nfunc Dynamic(c Caller) { c.Apply() }\n"
	if err := os.WriteFile(filepath.Join(dir, "identity.go"), []byte(src), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadCorpus(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewTypedVerifier(t.Context(), c.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	prefix := c.Snapshot.Package
	for _, tc := range []struct {
		caller, callee string
		want           bool
	}{
		{"CallA", "(A).Apply", true}, {"CallA", "(B).Apply", false}, {"Dynamic", "(A).Apply", false}, {"Dynamic", "(B).Apply", false},
	} {
		o, err := v.Verify(t.Context(), Relation{prefix + "." + tc.caller, prefix + "." + tc.callee}, 1, 0.999)
		if err != nil {
			t.Fatal(err)
		}
		if o.Confirmed != tc.want {
			t.Fatalf("wrong verification for %s -> %s", tc.caller, tc.callee)
		}
	}
	if _, err := v.Verify(t.Context(), Relation{prefix + ".CallA", "missing"}, 1, 0.99); err == nil {
		t.Fatal("unknown symbol was labeled as a negative")
	}
}
