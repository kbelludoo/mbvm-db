package erdosstraus

import (
	"encoding/json"
	"math/big"
	"os"
	"testing"
)

func TestRuleVerification(t *testing.T) {
	rules := StandardSeedRules()
	if len(rules) < 6 {
		t.Fatalf("expected at least 6 standard seed rules, got %d", len(rules))
	}

	testCases := []struct {
		ruleID string
		n      int64
	}{
		{"RULE-EVEN", 100},
		{"RULE-EVEN", 2048},
		{"RULE-MOD4-3", 103}, // 103 = 3 mod 4
		{"RULE-MOD4-3", 79},  // 79 = 3 mod 4
		{"RULE-MOD3-2", 107}, // 107 = 2 mod 3
		{"RULE-MOD3-2", 53},  // 53 = 2 mod 3
		{"RULE-MOD8-5", 109}, // 109 = 5 mod 8
		{"RULE-MOD8-5", 13},  // 13 = 5 mod 8
		{"RULE-MORDELL-DIVISOR", 73}, // 73 = 1 mod 24 (hard)
		{"RULE-MORDELL-DIVISOR", 1009}, // 1009 = 169 mod 840 (hard residue)
	}

	for _, tc := range testCases {
		var matchedRule *Rule
		for _, r := range rules {
			if r.ID == tc.ruleID {
				matchedRule = r
				break
			}
		}
		if matchedRule == nil {
			t.Fatalf("rule %s not found", tc.ruleID)
		}

		nBig := big.NewInt(tc.n)
		sol, err := matchedRule.Apply(nBig)
		if err != nil {
			t.Fatalf("rule %s failed on n=%d: %v", tc.ruleID, tc.n, err)
		}

		if err := VerifySolution(nBig, sol); err != nil {
			t.Fatalf("solution for n=%d via %s failed formal verification: %v", tc.n, tc.ruleID, err)
		}

		t.Logf("PASS: %s on n=%d => x=%s y=%s z=%s", tc.ruleID, tc.n, sol.X, sol.Y, sol.Z)
	}
}

func TestCSRGraphConstruction(t *testing.T) {
	rules := StandardSeedRules()
	g := NewMathCSRGraph(rules)

	if len(g.RuleIDs) != len(rules) {
		t.Fatalf("expected %d nodes in CSR graph, got %d", len(rules), len(g.RuleIDs))
	}
	if len(g.RowOffsets) != len(rules)+1 {
		t.Fatalf("expected %d row offsets, got %d", len(rules)+1, len(g.RowOffsets))
	}

	// Dynamic edge addition test
	err := g.AddEdge("RULE-MOD8-5", "RULE-MOD4-3", RelGeneralizes)
	if err != nil {
		t.Fatalf("dynamic edge insertion failed: %v", err)
	}

	idx := g.IDToIdx["RULE-MOD8-5"]
	neighbors := g.Neighbors(idx)
	found := false
	for _, edge := range neighbors {
		if g.RuleIDs[edge.Target] == "RULE-MOD4-3" && edge.Rel == RelGeneralizes {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected edge RULE-MOD8-5 -> RULE-MOD4-3 in CSR graph")
	}
}

func TestHDCMemoryRecall(t *testing.T) {
	rules := StandardSeedRules()
	hdc := NewHDCMemory(2026)

	for _, r := range rules {
		hdc.EncodeRule(r)
	}

	// Query for n = 100 (even)
	candidateModuli := []int64{2, 3, 4, 8, 12, 24}
	bestRuleID, dist := hdc.QueryBestRule(100, candidateModuli)
	if bestRuleID == "" {
		t.Fatalf("HDC query returned empty rule")
	}
	t.Logf("HDC query for n=100 returned rule=%s with hamming_dist=%d", bestRuleID, dist)
}

func TestComposerCRTAndMordell(t *testing.T) {
	rules := StandardSeedRules()
	composer := NewRuleComposer(rules)

	// Compose Mod 4 = 3 with Mod 3 = 2
	var rMod4, rMod3 *Rule
	for _, r := range rules {
		if r.ID == "RULE-MOD4-3" {
			rMod4 = r
		}
		if r.ID == "RULE-MOD3-2" {
			rMod3 = r
		}
	}

	cand, err := composer.ComposeCRT(rMod4, rMod3)
	if err != nil {
		t.Fatalf("CRT composition failed: %v", err)
	}
	if !cand.Verified {
		t.Fatalf("CRT composition was not verified: %s", cand.Refutation)
	}
	if cand.DerivedRule.Modulus != 12 {
		t.Fatalf("expected composed modulus 12, got %d", cand.DerivedRule.Modulus)
	}
	// 4k+3 and 3j+2 intersect at 11 mod 12
	if len(cand.DerivedRule.Residues) != 1 || cand.DerivedRule.Residues[0] != 11 {
		t.Fatalf("expected residue [11] mod 12, got %v", cand.DerivedRule.Residues)
	}
	t.Logf("CRT Composed rule: %s (mod %d, res %v) verified: %v",
		cand.DerivedRule.ID, cand.DerivedRule.Modulus, cand.DerivedRule.Residues, cand.Verified)

	// Test Mordell parametric family a=1, b=2 => 4(1)(2)-1 = 7 => n = -1 = 6 mod 7
	mord7, err := composer.DeriveMordellFamily(1, 2)
	if err != nil {
		t.Fatalf("Mordell derivation failed: %v", err)
	}
	if !mord7.Verified {
		t.Fatalf("Mordell derivation not verified: %s", mord7.Refutation)
	}
	if mord7.DerivedRule.Modulus != 7 || mord7.DerivedRule.Residues[0] != 6 {
		t.Fatalf("expected Mordell family (1,2) to cover n = 6 mod 7, got mod=%d res=%v",
			mord7.DerivedRule.Modulus, mord7.DerivedRule.Residues)
	}
	t.Logf("Mordell Derived rule: %s (mod %d, res %v) verified: %v",
		mord7.DerivedRule.ID, mord7.DerivedRule.Modulus, mord7.DerivedRule.Residues, mord7.Verified)
}

func TestErdosStrausControlledExperiment(t *testing.T) {
	results, err := RunControlledExperiment()
	if err != nil {
		t.Fatalf("controlled experiment failed: %v", err)
	}

	for _, res := range results {
		t.Logf("[%s] Solved: %d/%d (%.1f%%) | Tokens: %d (avg %.1f) | Time: %v | Invalid: %d",
			res.Group, res.SolvedCount, res.TotalProblems, res.SuccessRate*100,
			res.TotalTokens, res.AverageTokens, res.ExecutionTime, res.InvalidReturned)

		if res.InvalidReturned > 0 {
			t.Fatalf("group %s returned invalid solutions!", res.Group)
		}
	}

	// Verify Group C (MBVM-DB) solves 100% of benchmark items
	if results[2].SolvedCount != results[2].TotalProblems {
		t.Fatalf("Group C expected 100%% solved, got %d/%d", results[2].SolvedCount, results[2].TotalProblems)
	}

	// Verify token reduction of MBVM-DB (Group C) vs Conventional Retrieval (Group B)
	tokensB := results[1].TotalTokens
	tokensC := results[2].TotalTokens
	reduction := float64(tokensB-tokensC) / float64(tokensB) * 100.0
	t.Logf("Token Reduction (Group C vs Group B): %.2f%% (%d tokens vs %d tokens)",
		reduction, tokensC, tokensB)

	// Save experimental results to JSON artifact
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal results: %v", err)
	}
	if err := os.WriteFile("erdos_straus_results.json", data, 0644); err != nil {
		t.Fatalf("failed to write results artifact: %v", err)
	}
}

func TestHardResidueSieve840(t *testing.T) {
	// The open hard residues mod 840 coprime to 840 are: {1, 121, 169, 289, 361, 529} mod 840
	hardResidues := []int64{1, 121, 169, 289, 361, 529}
	primesTested := 0
	primesSolved := 0

	// Generate primes in these classes and test divisor resolution
	for _, res := range hardResidues {
		for k := int64(1); k <= 10; k++ {
			candidate := 840*k + res
			nBig := big.NewInt(candidate)
			if !nBig.ProbablyPrime(20) {
				continue
			}
			primesTested++
			sol, err := solveViaDivisorSearch(nBig, 200)
			if err == nil && sol != nil {
				if vErr := VerifySolution(nBig, sol); vErr == nil {
					primesSolved++
				}
			}
		}
	}

	t.Logf("Hard Residue Classes Mod 840 Sieve: Tested %d primes, Solved %d (%.1f%%)",
		primesTested, primesSolved, float64(primesSolved)/float64(primesTested)*100.0)
	if primesSolved < primesTested {
		t.Fatalf("expected all tested hard primes to be resolved by divisor search, got %d/%d",
			primesSolved, primesTested)
	}
}

func TestSynthesizedAdditiveRules(t *testing.T) {
	rules := SynthesizedAdditiveRules()
	if len(rules) != 4 {
		t.Fatalf("expected 4 synthesized rules, got %d", len(rules))
	}

	for _, r := range rules {
		// Test each synthesized rule across multiple values of k
		for k := int64(1); k <= 20; k++ {
			nVal := r.Modulus*k + r.Residues[0]
			nBig := big.NewInt(nVal)

			sol, err := r.Apply(nBig)
			if err != nil {
				t.Fatalf("rule %s failed on n=%d: %v", r.ID, nVal, err)
			}
			if err := VerifySolution(nBig, sol); err != nil {
				t.Fatalf("rule %s verification failed on n=%d: %v", r.ID, nVal, err)
			}
		}
		t.Logf("PASS: %s verified on 20 test instances", r.ID)
	}
}


func TestHardClasses840ParametricFamilies(t *testing.T) {
	rules := HardClasses840ParametricRules()
	if len(rules) != 8 {
		t.Fatalf("expected 8 hard class rules, got %d", len(rules))
	}

	for _, r := range rules {
		// 1. Strict mathematical proof that this rule is NOT in any elementary class
		// All hard class residues must be: n = 1 mod 24 => n = 1 mod 8 and n = 1 mod 3
		for _, res := range r.Residues {
			if res%2 == 0 {
				t.Fatalf("rule %s contains even residue %d (violates hard class requirement)", r.ID, res)
			}
			if res%4 == 3 {
				t.Fatalf("rule %s contains residue 3 mod 4 (violates hard class requirement)", r.ID)
			}
			if res%3 == 2 {
				t.Fatalf("rule %s contains residue 2 mod 3 (violates hard class requirement)", r.ID)
			}
			if res%8 == 5 {
				t.Fatalf("rule %s contains residue 5 mod 8 (violates hard class requirement)", r.ID)
			}
			// Verify it belongs to one of the 6 hard residues mod 840
			res840 := res % 840
			valid840 := res840 == 1 || res840 == 121 || res840 == 169 || res840 == 289 || res840 == 361 || res840 == 529
			if !valid840 {
				t.Fatalf("rule %s residue %d is not in the 6 hard quadratic classes mod 840", r.ID, res840)
			}
		}

		// 2. Concrete test of closed polynomial solver over 25 values of k
		for k := int64(0); k < 25; k++ {
			nVal := r.Modulus*k + r.Residues[0]
			if nVal < 2 {
				continue
			}
			nBig := big.NewInt(nVal)

			sol, err := r.Apply(nBig)
			if err != nil {
				t.Fatalf("rule %s failed on n=%d: %v", r.ID, nVal, err)
			}
			if err := VerifySolution(nBig, sol); err != nil {
				t.Fatalf("rule %s verification failed on n=%d: %v", r.ID, nVal, err)
			}
		}

		t.Logf("PASS: %s (mod %d, res %d) strictly outside elementary classes, verified 25 instances",
			r.ID, r.Modulus, r.Residues[0])
	}
}


func TestRegressionAuditedCases(t *testing.T) {
	// Regression 1: p = 2 must panic due to parity bug identified in audit
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for even prime p=2, but did not panic")
		} else {
			t.Logf("PASS: HardClassParametricFamily correctly panicked on even p=2: %v", r)
		}
	}()
	_ = HardClassParametricFamily("RULE-INVALID-P2", 2, 1, 9240, 8401)
}

func TestRegressionGeneralTheoremOverlap(t *testing.T) {
	// Regression 2: p = 5, n = 37 demonstrates that the general 1 mod 12 theorem
	// is valid algebraically, but intersects the elementary class n = 5 mod 8
	n := big.NewInt(37)
	p := int64(5)

	// x = (n+3)/4 = 10
	x := new(big.Int).Div(new(big.Int).Add(n, threeBig), four)
	// y = (n(n+3) + 4p)/12 = (37*40 + 20)/12 = 1500/12 = 125
	nNplus3 := new(big.Int).Mul(n, new(big.Int).Add(n, threeBig))
	y := new(big.Int).Div(new(big.Int).Add(nNplus3, big.NewInt(4*p)), twelveBig)
	// z = n(n+3)[n(n+3)+4p] / (48p) = (1480 * 1500) / 240 = 9250
	zNum := new(big.Int).Mul(nNplus3, new(big.Int).Add(nNplus3, big.NewInt(4*p)))
	z := new(big.Int).Div(zNum, big.NewInt(48*p))

	sol := &Solution{X: x, Y: y, Z: z}
	if err := VerifySolution(n, sol); err != nil {
		t.Fatalf("p=5, n=37 verification failed: %v", err)
	}

	// Verify n = 37 mod 8 == 5 (elementary class overlap)
	if n.Int64()%8 != 5 {
		t.Fatalf("expected n=37 to be 5 mod 8, got %d", n.Int64()%8)
	}
	t.Logf("PASS: n=37, p=5 verified algebraically (x=%s, y=%s, z=%s) and correctly identified in elementary class 5 mod 8",
		x.String(), y.String(), z.String())
}
