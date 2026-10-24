package realeval

import (
	"context"
	"os"
	"testing"
)

func TestUnseenDatasetIntegrity(t *testing.T) {
	ds := UnseenEvaluationDataset()
	if len(ds) != 10 {
		t.Fatalf("expected 10 unseen challenge problems, got %d", len(ds))
	}

	seenIDs := make(map[string]bool)
	for _, ch := range ds {
		if seenIDs[ch.ID] {
			t.Fatalf("duplicate challenge ID: %s", ch.ID)
		}
		seenIDs[ch.ID] = true
		if ch.N == nil || ch.N.Sign() <= 0 {
			t.Fatalf("invalid challenge n for %s", ch.ID)
		}
	}
	t.Logf("PASS: 10 unseen challenge problems configured and validated")
}

func TestPromptGenerationConditions(t *testing.T) {
	ds := UnseenEvaluationDataset()
	ch := ds[0]

	sysA, userA := buildPrompts(ConditionDirectBaseline, ch)
	sysB, userB := buildPrompts(ConditionConventionalRAG, ch)
	sysC, userC := buildPrompts(ConditionMBVMSliced, ch)

	if len(sysB) <= len(sysA) {
		t.Fatalf("expected Flat RAG prompt to be larger than Direct Baseline")
	}
	if len(sysC) >= len(sysB) {
		t.Fatalf("expected MBVM Sliced prompt to be smaller than Flat RAG")
	}
	t.Logf("Prompt lengths: Direct=%d chars | FlatRAG=%d chars | MBVM-Sliced=%d chars (User=%s)",
		len(sysA)+len(userA), len(sysB)+len(userB), len(sysC)+len(userC), ch.ID)
}

func TestExtractXYZParser(t *testing.T) {
	sample := "The solution is given by: x = 20, y = 219, z = 4380."
	x, y, z := extractXYZ(sample)
	if x != "20" || y != "219" || z != "4380" {
		t.Fatalf("parser failed: got x=%s y=%s z=%s", x, y, z)
	}
	t.Logf("PASS: parser extracted x=%s, y=%s, z=%s", x, y, z)
}

func TestLiveBenchmarkClientRequirement(t *testing.T) {
	// If GEMINI_API_KEY is not set, NewGeminiClient must fail gracefully with ErrMissingAPIKey
	if os.Getenv("GEMINI_API_KEY") == "" && os.Getenv("GOOGLE_API_KEY") == "" {
		_, err := NewGeminiClient("gemini-2.5-flash")
		if err == nil {
			t.Fatalf("expected error when API key is missing")
		}
		t.Logf("PASS: NewGeminiClient correctly enforces missing API key requirement: %v", err)
		return
	}

	// If API key is present, test a real ping call
	client, err := NewGeminiClient("gemini-2.5-flash")
	if err != nil {
		t.Fatalf("client initialization failed: %v", err)
	}
	ctx := context.Background()
	resp, err := client.GenerateContent(ctx, "Answer with one word: 'READY'", "Ping")
	if err != nil {
		t.Logf("live ping skipped or errored (network/quota): %v", err)
		return
	}
	t.Logf("LIVE PING SUCCESS: PromptTokens=%d, CandidatesTokens=%d",
		resp.UsageMetadata.PromptTokenCount, resp.UsageMetadata.CandidatesTokenCount)
}
