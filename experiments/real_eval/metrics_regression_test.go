package realeval

import (
	"testing"
	"time"
)

func TestAblationSummaryExcludesInfrastructureErrorsFromTokenAverages(t *testing.T) {
	results := []AblationTrialResult{
		{
			Condition:            ConditionAblationFormulaOnly,
			Success:              true,
			PromptTokensReal:     200,
			CompletionTokensReal: 597,
			ReasoningTokensReal:  3394,
			TotalTokensReal:      4191,
			Latency:              17 * time.Second,
		},
		{
			Condition:    ConditionAblationFormulaOnly,
			ErrorMessage: "HTTP 503: unavailable",
			Latency:      30 * time.Second,
		},
	}
	summaries := computeAblationSummaries(results)
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want one", len(summaries))
	}
	got := summaries[0]
	if got.TrialsCount != 2 || got.CompletedResponses != 1 || got.InfrastructureErrors != 1 {
		t.Fatalf("incorrect response/error counts: %+v", got)
	}
	if got.SolvedCount != 1 || got.MathFailuresCount != 0 {
		t.Fatalf("incorrect mathematical results: %+v", got)
	}
	if got.AvailabilityRate != 0.5 || got.MathematicalAccuracy != 1 {
		t.Fatalf("incorrect availability/accuracy: %+v", got)
	}
	if got.AvgTotalTokens != 4191 || got.AvgReasoningTok != 3394 || got.AvgPromptTokens != 200 {
		t.Fatalf("HTTP 503 incorrectly diluted token averages: %+v", got)
	}
	if got.AvgLatencyMs != 17000 {
		t.Fatalf("HTTP 503 incorrectly diluted latency average: %+v", got)
	}
}

func TestBenchmarkSummaryExcludesInfrastructureErrors(t *testing.T) {
	results := []TrialResult{
		{Condition: ConditionMBVMCSR, Success: false, TotalTokensReal: 750, Latency: time.Second},
		{Condition: ConditionMBVMCSR, ErrorMessage: "HTTP 503: unavailable", Latency: 10 * time.Second},
		{Condition: ConditionMBVMCSR, Success: true, TotalTokensReal: 1250, Latency: 3 * time.Second},
	}
	summaries := computeSummaries(results)
	if len(summaries) != 1 {
		t.Fatalf("got %d summaries, want one", len(summaries))
	}
	got := summaries[0]
	if got.TotalProblems != 3 || got.CompletedResponses != 2 || got.InfrastructureErrors != 1 {
		t.Fatalf("incorrect response/error counts: %+v", got)
	}
	if got.SolvedCount != 1 || got.Accuracy != 0.5 || got.AvailabilityRate != 2.0/3.0 {
		t.Fatalf("incorrect accuracy or availability: %+v", got)
	}
	if got.AvgTotalTokens != 1000 || got.AvgLatencyMs != 2000 {
		t.Fatalf("HTTP 503 incorrectly diluted averages: %+v", got)
	}
}
