package realeval

import (
	"math/big"
	"testing"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

func TestCRTDepthDevelopmentThreePremiseParity(t *testing.T) {
	lab := NewCRTDepthLab()
	if len(lab.Tasks) != 12 { t.Fatalf("want 12 public development tasks, got %d", len(lab.Tasks)) }
	if len(lab.Catalog) != 12*7 { t.Fatalf("want 84 catalog nodes including distractors, got %d", len(lab.Catalog)) }

	results, err := lab.RunDepthPaired()
	if err != nil { t.Fatal(err) }
	if len(results) != len(lab.Tasks) { t.Fatalf("got %d results", len(results)) }

	for i, result := range results {
		task := lab.Tasks[i]
		if !result.SameContext || !result.SameCertificate ||
			!result.Conventional.Certified || !result.CSR.Certified {
			t.Fatalf("%s: B/C multi-hop retrieval differs: %+v", task.ID, result)
		}
		if len(result.CSR.RetrievedIDs) != 3 || len(result.Conventional.RetrievedIDs) != 3 {
			t.Fatalf("%s: did not retrieve exactly three indispensable premises", task.ID)
		}
		for j, id := range result.CSR.RetrievedIDs {
			if id != result.Conventional.RetrievedIDs[j] { t.Fatalf("%s: rule order differs", task.ID) }
			for _, noiseID := range task.NoiseIDs {
				if id == noiseID { t.Fatalf("%s: irrelevant rule %s reached", task.ID, id) }
			}
		}
		final := result.CSR.Certificate.Final
		if final == nil { t.Fatalf("%s: missing final CRT stage", task.ID) }
		R, okR := new(big.Int).SetString(final.Residue, 10)
		M, okM := new(big.Int).SetString(final.Modulus, 10)
		if !okR || !okM || !R.IsInt64() || !M.IsInt64() { t.Fatalf("%s: malformed CRT class", task.ID) }

		// Independent, exhaustive finite oracle for these small moduli.
		// Removing any one premise must make the unique class underdetermined.
		mods := [3]int64{}
		rems := [3]int64{}
		expectedM := int64(1)
		for j, id := range task.LeafIDs {
			r := lab.byID[id]
			mods[j], rems[j] = r.Modulus, r.Residues[0]
			expectedM *= r.Modulus
		}
		if M.Int64() != expectedM { t.Fatalf("%s: incorrect combined modulus %s", task.ID, M) }
		var counts [4]int64
		for n := int64(0); n < expectedM; n++ {
			valid := [3]bool{}
			for j := 0; j < 3; j++ { valid[j] = n%mods[j] == rems[j] }
			if valid[0] && valid[1] && valid[2] {
				counts[0]++
				if n != R.Int64() { t.Errorf("%s: wrong unique residue: %d, want %s", task.ID, n, R) }
			}
			for omit := 0; omit < 3; omit++ {
				eligible := true
				for j := 0; j < 3; j++ {
					if j != omit && !valid[j] { eligible = false }
				}
				if eligible { counts[omit+1]++ }
			}
		}
		if counts[0] != 1 { t.Fatalf("%s: unique class count=%d", task.ID, counts[0]) }
		for omit := 0; omit < 3; omit++ {
			if counts[omit+1] != mods[omit] {
				t.Fatalf("%s: removing premise %d produced %d solutions, want %d", task.ID, omit, counts[omit+1], mods[omit])
			}
		}
	}
}

func TestCRTDepthActuallyTraversesTwoDependencyLevels(t *testing.T) {
	lab := NewCRTDepthLab()
	for _, task := range lab.Tasks {
		root := lab.Graph.IDToIdx[task.GoalID]
		mid := lab.Graph.IDToIdx[task.MidID]
		direct := make(map[string]bool)
		for _, e := range lab.Graph.Neighbors(root) {
			if e.Rel == erdosstraus.RelDependency { direct[lab.Graph.RuleIDs[e.Target]] = true }
		}
		if !direct[task.MidID] || !direct[task.LeafIDs[2]] || len(direct) != 2 {
			t.Fatalf("%s: root dependencies not [MID, L3]: %+v", task.ID, direct)
		}
		for _, id := range task.LeafIDs[:2] {
			if direct[id] { t.Fatalf("%s: %s was flattened into a direct root dependency", task.ID, id) }
		}
		midDeps := make(map[string]bool)
		for _, e := range lab.Graph.Neighbors(mid) {
			if e.Rel == erdosstraus.RelDependency { midDeps[lab.Graph.RuleIDs[e.Target]] = true }
		}
		if !midDeps[task.LeafIDs[0]] || !midDeps[task.LeafIDs[1]] {
			t.Fatalf("%s: intermediate dependency chain incomplete: %+v", task.ID, midDeps)
		}
	}
}

func TestCRTDepthRequiresAllThreeLeavesAndRejectsForgedProof(t *testing.T) {
	lab := NewCRTDepthLab()
	task := lab.Tasks[0]
	rules, err := lab.retrieveDepthCSR(task.GoalID)
	if err != nil { t.Fatal(err) }
	if _, err := lab.composeDepth(task, rules[:2]); err == nil {
		t.Fatal("two of three leaves incorrectly yielded full CRT certificate")
	}
	proof, err := lab.composeDepth(task, rules)
	if err != nil { t.Fatal(err) }
	copyRule := &erdosstraus.Rule{ID: task.LeafIDs[2], Modulus: 5, Residues: []int64{4}}
	forgedRules := append([]*erdosstraus.Rule(nil), rules...)
	for i, r := range forgedRules {
		if r.ID == task.LeafIDs[2] { forgedRules[i] = copyRule }
	}
	// Modified leaf is not allowed to silently retain the original certificate.
	changed, err := lab.composeDepth(task, forgedRules)
	if err == nil && changed.Final.Residue == proof.Final.Residue {
		t.Fatal("altering a required residue did not affect CRT proof")
	}
}
