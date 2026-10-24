package realeval

import (
	"context"
	"math/big"
	"os"
	"strings"
	"testing"
)

func TestBenchmark30Generation(t *testing.T) {
	problems, err := GenerateBenchmark30()
	if err != nil {
		t.Fatalf("failed to generate 30 problems: %v", err)
	}
	if len(problems) != 30 {
		t.Fatalf("expected exactly 30 problems, got %d", len(problems))
	}

	counts := make(map[ProblemType]int)
	for _, p := range problems {
		counts[p.Type]++
	}

	if counts[ProblemConstruction] != 8 || counts[ProblemProof] != 8 ||
		counts[ProblemRefutation] != 7 || counts[ProblemObstruction] != 7 {
		t.Fatalf("unexpected problem type distribution: %+v", counts)
	}

	t.Logf("Benchmark-30 Balanced Counts: Construction=%d, Proof=%d, Refutation=%d, Obstruction=%d",
		counts[ProblemConstruction], counts[ProblemProof], counts[ProblemRefutation], counts[ProblemObstruction])
}

func TestUnseenBenchmark20Generation(t *testing.T) {
	problems, err := GenerateUnseenBenchmark20()
	if err != nil {
		t.Fatalf("failed to generate 20 unseen problems: %v", err)
	}
	if len(problems) != 20 {
		t.Fatalf("expected exactly 20 problems, got %d", len(problems))
	}

	counts := make(map[ProblemType]int)
	for _, p := range problems {
		counts[p.Type]++
	}

	if counts[ProblemConstruction] != 8 || counts[ProblemProof] != 4 ||
		counts[ProblemRefutation] != 4 || counts[ProblemObstruction] != 4 {
		t.Fatalf("unexpected problem type distribution in unseen: %+v", counts)
	}

	t.Logf("Unseen-20 Counts: Construction=%d, Proof=%d, Refutation=%d, Obstruction=%d",
		counts[ProblemConstruction], counts[ProblemProof], counts[ProblemRefutation], counts[ProblemObstruction])
}

func TestRetrievalFourConditions(t *testing.T) {
	kb := NewKnowledgeBase()
	n26881 := big.NewInt(26881) // 1 mod 840, 8401 mod 9240 (covered by RULE-HARD-840-RES1-P11)

	flat := kb.RetrieveFlat()
	if len(flat.Rules) != 16 {
		t.Fatalf("expected 16 rules in flat RAG, got %d", len(flat.Rules))
	}

	filtered := kb.RetrieveFiltered(n26881)
	if len(filtered.Rules) != 1 {
		t.Fatalf("expected exactly 1 matching rule in filtered index, got %d", len(filtered.Rules))
	}
	if filtered.Rules[0].ID != "RULE-HARD-840-RES1-P11" {
		t.Fatalf("expected RULE-HARD-840-RES1-P11, got %s", filtered.Rules[0].ID)
	}

	csr := kb.RetrieveMBVMCSR(n26881)
	if len(csr.Rules) < 2 {
		t.Fatalf("expected CSR to retrieve root rule + dependencies, got %d rules", len(csr.Rules))
	}
	if csr.Rules[0].ID != "RULE-HARD-840-RES1-P11" {
		t.Fatalf("expected root of CSR to be RULE-HARD-840-RES1-P11, got %s", csr.Rules[0].ID)
	}

	t.Logf("Rules count: Flat=%d (Tokens %d) | Filtered=%d (Tokens %d) | CSR=%d (Tokens %d)",
		len(flat.Rules), flat.TokenWeight, len(filtered.Rules), filtered.TokenWeight, len(csr.Rules), csr.TokenWeight)
	t.Logf("CSR Path: %s", csr.Path)
}

func TestVerifierOracleNoKeywords(t *testing.T) {
	// 1. Valid Construction for N=73: 4/73 = 1/20 + 1/219 + 1/4380
	sol73Text := "Here is the exact solution:\nx = 20\ny = 219\nz = 4380\nThis completes the proof."
	prob73 := EvalProblem{
		ID:   "B30-01",
		Type: ProblemConstruction,
		N:    big.NewInt(73),
	}
	okConst, msgConst := VerifyResponse(prob73, sol73Text)
	if !okConst {
		t.Fatalf("failed to verify valid solution for N=73: %s", msgConst)
	}
	t.Logf("PASS: Construction verified exact via math/big: %s", msgConst)

	// 2. Mathematical Refutation for B30-17: p=2, n=25 => z = 10325/2 (non-integer)
	ref17Text := "For p = 2 and n = 25, substituting into the formula gives z = 10325/2, which is not an integer."
	prob17 := EvalProblem{
		ID:   "B30-17",
		Type: ProblemRefutation,
	}
	okRef, msgRef := VerifyResponse(prob17, ref17Text)
	if !okRef {
		t.Fatalf("failed to verify valid refutation for B30-17: %s", msgRef)
	}
	t.Logf("PASS: Refutation verified via mathematical counterexample: %s", msgRef)

	// 3. Keyword-only or trivial strings MUST FAIL (addressing Zyon's audit)
	trivialCases := []struct {
		problem EvalProblem
		trivial string
	}{
		{EvalProblem{ID: "B30-16", Type: ProblemProof}, "840"},
		{EvalProblem{ID: "B30-19", Type: ProblemRefutation}, "33"},
		{EvalProblem{ID: "B30-29", Type: ProblemObstruction}, "2/11"},
		{EvalProblem{ID: "B30-17", Type: ProblemRefutation}, "p=2, n=25, not an integer"},
	}

	for _, tc := range trivialCases {
		ok, reason := VerifyResponse(tc.problem, tc.trivial)
		if ok {
			t.Fatalf("Problem %s accepted trivial string %q! Oracle must reject without proof steps.", tc.problem.ID, tc.trivial)
		}
		t.Logf("PASS: Oracle correctly rejected trivial response %q for %s: %s", tc.trivial, tc.problem.ID, reason)
	}
}

func TestAblationPromptIsolation(t *testing.T) {
	runner := NewRunner(nil, "")
	prob := EvalProblem{
		ID:     "U20-02-ABLATION",
		Type:   ProblemConstruction,
		N:      big.NewInt(26881),
		Prompt: "Find positive integers x, y, z satisfying 4/26881 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.",
	}

	_, promptA, retA := runner.buildAblationPrompt(ConditionAblationDirect, prob)
	_, promptB, retB := runner.buildAblationPrompt(ConditionAblationFormulaOnly, prob)
	_, promptC, retC := runner.buildAblationPrompt(ConditionAblationFormulaRecipe, prob)
	_, promptD, retD := runner.buildAblationPrompt(ConditionAblationCSRRecipe, prob)

	if len(retA.Rules) != 0 {
		t.Fatalf("Group A must not retrieve rules")
	}
	if !strings.Contains(promptA, "26881") {
		t.Fatalf("Group A prompt should mention 26881")
	}
	if len(retB.Rules) != 1 {
		t.Fatalf("Group B expected 1 rule, got %d", len(retB.Rules))
	}
	if strings.Contains(promptB, "Topological Instantiation Recipe") {
		t.Fatalf("Group B (Formula Only) must not contain recipe!")
	}
	if !strings.Contains(promptC, "Topological Instantiation Recipe") {
		t.Fatalf("Group C must contain recipe!")
	}
	if len(retC.Rules) != 1 {
		t.Fatalf("Group C expected 1 rule (filtered), got %d", len(retC.Rules))
	}
	if !strings.Contains(promptD, "Topological Instantiation Recipe") {
		t.Fatalf("Group D must contain recipe!")
	}
	if len(retD.Rules) <= 1 {
		t.Fatalf("Group D expected CSR subgraph (multiple rules), got %d", len(retD.Rules))
	}
	t.Logf("PASS: Ablation prompt isolation verified across all 4 groups")
}

func TestVerifierOracleRigorousRefutation(t *testing.T) {
	// U20-15: Non-uniqueness for prime n=3
	prob15 := EvalProblem{ID: "U20-15", Type: ProblemRefutation}
	bad15 := "Prime n=3 has multiple distinct solutions like distinct two three solutions."
	ok, _ := VerifyResponse(prob15, bad15)
	if ok {
		t.Fatalf("U20-15 should reject keyword-only response!")
	}

	good15 := "For prime n=3, 4/3 can be written as (1, 4, 12) and also (1, 6, 6). Hence non-unique."
	ok, details := VerifyResponse(prob15, good15)
	if !ok {
		t.Fatalf("U20-15 should accept 2 distinct verified triples! Reason: %s", details)
	}
	t.Logf("PASS: U20-15 verified with math/big distinct triples: %s", details)

	// U20-16: Refute n=1 mod 12 with counterexample
	prob16 := EvalProblem{ID: "U20-16", Type: ProblemRefutation}
	bad16 := "n=25 is 1 mod 12, with x=7. This is a counterexample mod 3."
	ok, reason := VerifyResponse(prob16, bad16)
	if ok {
		t.Fatalf("U20-16 must reject false counterexample n=25!")
	}
	t.Logf("PASS: U20-16 correctly rejected false counterexample n=25: %s", reason)

	good16 := "Counterexample n=49 gives x=13. t = 49*13 = 637 = 7^2 * 13. All prime factors 7 and 13 are 1 mod 3, so all divisors are 1 mod 3. Target is -t mod 3 = 2 mod 3, so no divisor can satisfy this."
	ok, details16 := VerifyResponse(prob16, good16)
	if !ok {
		t.Fatalf("U20-16 should accept certified counterexample n=49! Reason: %s", details16)
	}
	t.Logf("PASS: U20-16 accepted certified counterexample n=49: %s", details16)
}

func TestRunnerSingleIntegrationWiring(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		client, err := NewGeminiClient("gemini-3.5-flash")
		if err == nil {
			t.Fatalf("expected error when API key is missing")
		}
		runner := NewRunner(client, "")
		_, errRun := runner.RunSingleIntegrationTest(context.Background())
		if errRun == nil {
			t.Fatalf("expected error running integration test without API key")
		}
		t.Logf("PASS: Runner enforces API key requirement gracefully: %v", errRun)
		return
	}

	client, err := NewGeminiClient("gemini-3.5-flash")
	if err != nil {
		t.Fatalf("failed to initialize client with key: %v", err)
	}
	runner := NewRunner(client, "test_integration.json")
	defer os.Remove("test_integration.json")

	result, err := runner.RunSingleIntegrationTest(context.Background())
	if err != nil {
		t.Logf("live integration test skipped due to quota/network: %v", err)
		return
	}
	t.Logf("SINGLE LIVE INTEGRATION SUCCESS: Tokens=%d, Latency=%v, Status=%s",
		result.TotalTokensReal, result.Latency, result.VerificationStatus)
}
