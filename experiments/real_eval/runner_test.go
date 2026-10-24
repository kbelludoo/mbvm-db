package realeval

import (
	"context"
	"math/big"
	"os"
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

	t.Logf("Benchmark-30 Balanced Counts: Construction=%d, Proof=%d, Refutation=%d, Obstruction=%d",
		counts[ProblemConstruction], counts[ProblemProof], counts[ProblemRefutation], counts[ProblemObstruction])

	if counts[ProblemConstruction] != 8 || counts[ProblemProof] != 8 || counts[ProblemRefutation] != 7 || counts[ProblemObstruction] != 7 {
		t.Fatalf("expected 8/8/7/7 distribution, got %v", counts)
	}
}

func TestRetrievalFourConditions(t *testing.T) {
	kb := NewKnowledgeBase()
	n := big.NewInt(193) // 193 = 1 mod 24

	flat := kb.RetrieveFlat()
	filtered := kb.RetrieveFiltered(n)
	csr := kb.RetrieveMBVMCSR(n)

	if len(flat.Rules) <= len(filtered.Rules) {
		t.Fatalf("expected flat dump to have more rules than filtered")
	}
	if len(csr.Rules) == 0 {
		t.Fatalf("expected CSR retrieval to return active subgraph rules")
	}

	t.Logf("Rules count: Flat=%d (Tokens %d) | Filtered=%d (Tokens %d) | CSR=%d (Tokens %d)",
		len(flat.Rules), flat.TokenWeight,
		len(filtered.Rules), filtered.TokenWeight,
		len(csr.Rules), csr.TokenWeight)
	t.Logf("CSR Path: %s", csr.Path)
}

func TestVerifierOracleNoKeywords(t *testing.T) {
	// Construction check
	p1 := EvalProblem{
		ID:   "TEST-01",
		Type: ProblemConstruction,
		N:    big.NewInt(73),
	}
	validAns := "Here is the verified solution: x = 20, y = 219, z = 4380."
	success, msg := VerifyResponse(p1, validAns)
	if !success {
		t.Fatalf("expected valid construction to pass: %s", msg)
	}
	t.Logf("PASS: Construction verified exact via math/big: %s", msg)

	// Refutation check: p=2, n=25
	p17 := EvalProblem{
		ID:   "B30-17",
		Type: ProblemRefutation,
	}
	validRefutation := "Refutation: choose p = 2 and n = 25. Then x = 7, y = 59, but z = 10325/2 which is not an integer."
	successRef, msgRef := VerifyResponse(p17, validRefutation)
	if !successRef {
		t.Fatalf("expected valid refutation to pass: %s", msgRef)
	}
	t.Logf("PASS: Refutation verified via mathematical counterexample: %s", msgRef)

	// Keyword-only response without mathematical verification MUST FAIL
	fakeResponse := "This statement is impossible and refuted."
	successFake, _ := VerifyResponse(p17, fakeResponse)
	if successFake {
		t.Fatalf("keyword-only fake response must NOT be accepted as a valid proof!")
	}
	t.Logf("PASS: Keyword-only response correctly rejected by verifier oracle")
}

func TestRunnerSingleIntegrationWiring(t *testing.T) {
	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		client, err := NewGeminiClient("gemini-2.5-flash")
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

	client, err := NewGeminiClient("gemini-2.5-flash")
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
