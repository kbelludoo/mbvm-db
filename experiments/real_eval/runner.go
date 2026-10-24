package realeval

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// Runner manages live execution across all conditions with real Gemini API calls.
type Runner struct {
	Client        *GeminiClient
	KB            *KnowledgeBase
	ReportPath    string
	RNG           *rand.Rand
}

// NewRunner initializes the runner with client, knowledge base, and CSR structures.
func NewRunner(client *GeminiClient, reportPath string) *Runner {
	if reportPath == "" {
		reportPath = "experiments/real_eval/live_benchmark_results.json"
	}
	return &Runner{
		Client:     client,
		KB:         NewKnowledgeBase(),
		ReportPath: reportPath,
		RNG:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RunSingleIntegrationTest executes a single test problem to verify API wiring before consuming full quota.
func (r *Runner) RunSingleIntegrationTest(ctx context.Context) (*TrialResult, error) {
	if r.Client == nil {
		return nil, fmt.Errorf("client cannot be nil: %w", ErrMissingAPIKey)
	}

	testProb := EvalProblem{
		ID:     "INTEGRATION-01",
		Type:   ProblemConstruction,
		Prompt: "Find positive integers x, y, z satisfying 4/5 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.",
	}

	retrieval := r.KB.RetrieveMBVMCSR(nil)
	sysPrompt := "You are a mathematical theorem prover. State: x = <val>, y = <val>, z = <val>."
	userPrompt := fmt.Sprintf("%s\n\nContext:\n%s", testProb.Prompt, formatRules(retrieval.Rules))

	start := time.Now()
	resp, err := r.Client.GenerateContent(ctx, sysPrompt, userPrompt)
	latency := time.Since(start)

	if err != nil {
		return nil, fmt.Errorf("single integration test call failed: %w", err)
	}

	var ansText string
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		ansText = resp.Candidates[0].Content.Parts[0].Text
	}

	success, details := verifyConstruction(testProb, ansText)

	trial := &TrialResult{
		ProblemID:            testProb.ID,
		Condition:            ConditionMBVMCSR,
		PromptTokensReal:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokensReal: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokensReal:      resp.UsageMetadata.TotalTokenCount,
		ReasoningTokensReal:  resp.UsageMetadata.CandidatesTokenDetails.ReasoningTokenCount,
		Latency:              latency,
		Success:              success,
		VerificationStatus:   details,
		RetrievalPath:        retrieval.Path,
		RulesRetrieved:       retrieval.RuleIDs,
		ModelVersion:         resp.ModelVersion,
		FullResponse:         ansText,
	}

	return trial, nil
}

// RunFullBenchmark executes the 30-problem suite across the 4 conditions with randomized ordering.
func (r *Runner) RunFullBenchmark(ctx context.Context) ([]TrialResult, []BenchmarkSummary, error) {
	problems, err := GenerateBenchmark30()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate 30 problems: %w", err)
	}

	allConditions := []ConditionType{
		ConditionDirectBaseline,
		ConditionFlatRAG,
		ConditionFilteredRAG,
		ConditionMBVMCSR,
	}

	results := make([]TrialResult, 0, len(problems)*4)

	for _, prob := range problems {
		// Randomize condition order per problem to eliminate systemic order bias
		conds := make([]ConditionType, len(allConditions))
		copy(conds, allConditions)
		r.RNG.Shuffle(len(conds), func(i, j int) { conds[i], conds[j] = conds[j], conds[i] })

		for _, cond := range conds {
			sysPrompt, userPrompt, ret := r.buildPrompt(cond, prob)

			start := time.Now()
			resp, callErr := r.Client.GenerateContent(ctx, sysPrompt, userPrompt)
			latency := time.Since(start)

			trial := TrialResult{
				ProblemID:     prob.ID,
				Condition:     cond,
				Latency:       latency,
				RetrievalPath: ret.Path,
				RulesRetrieved: ret.RuleIDs,
			}

			if callErr != nil {
				trial.ErrorMessage = callErr.Error()
				trial.VerificationStatus = "API call failed"
				results = append(results, trial)
				continue
			}

			trial.PromptTokensReal = resp.UsageMetadata.PromptTokenCount
			trial.CompletionTokensReal = resp.UsageMetadata.CandidatesTokenCount
			trial.TotalTokensReal = resp.UsageMetadata.TotalTokenCount
			trial.ReasoningTokensReal = resp.UsageMetadata.CandidatesTokenDetails.ReasoningTokenCount
			trial.ModelVersion = resp.ModelVersion

			var ansText string
			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				ansText = resp.Candidates[0].Content.Parts[0].Text
			}
			trial.FullResponse = ansText

			// Strict mathematical verification (no keyword matching)
			success, status := VerifyResponse(prob, ansText)
			trial.Success = success
			trial.VerificationStatus = status

			x, y, z := extractXYZ(ansText)
			trial.ExtractedX = x
			trial.ExtractedY = y
			trial.ExtractedZ = z

			results = append(results, trial)
		}
	}

	// Compute summaries
	summaries := computeSummaries(results)

	// Persist report and explicitly check error
	payload := map[string]any{
		"results":   results,
		"summaries": summaries,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal benchmark results: %w", err)
	}

	if err := os.WriteFile(r.ReportPath, raw, 0644); err != nil {
		return nil, nil, fmt.Errorf("failed to write benchmark results file to %s: %w", r.ReportPath, err)
	}

	return results, summaries, nil
}

func (r *Runner) buildPrompt(cond ConditionType, prob EvalProblem) (string, string, RetrievalResult) {
	sysBase := "You are an expert mathematical researcher. Provide a complete, rigorous proof or exact numbers."

	switch cond {
	case ConditionDirectBaseline:
		return sysBase, prob.Prompt, RetrievalResult{Path: "Direct(NoMemory)"}

	case ConditionFlatRAG:
		ret := r.KB.RetrieveFlat()
		user := fmt.Sprintf("%s\n\nReference Rules (Unfiltered Full Knowledge Base):\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysBase, user, ret

	case ConditionFilteredRAG:
		ret := r.KB.RetrieveFiltered(prob.N)
		user := fmt.Sprintf("%s\n\nReference Rules (Conventional Inverted Index Matched):\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysBase, user, ret

	case ConditionMBVMCSR:
		ret := r.KB.RetrieveMBVMCSR(prob.N)
		user := fmt.Sprintf("%s\n\nMBVM-DB Smart Sliced Active Subgraph:\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysBase, user, ret

	default:
		return sysBase, prob.Prompt, RetrievalResult{}
	}
}

func formatRules(rules []*erdosstraus.Rule) string {
	var sb strings.Builder
	for _, r := range rules {
		sb.WriteString(fmt.Sprintf("- [%s] %s: Hypotheses=%v, Conclusion=%s\n  Proof=%s\n",
			r.ID, r.Name, r.Hypotheses, r.Conclusion, r.ProofText))
	}
	return sb.String()
}

func computeSummaries(results []TrialResult) []BenchmarkSummary {
	type accum struct {
		total, solved int
		promptTok, compTok, totalTok int
		latency time.Duration
	}
	acc := make(map[ConditionType]*accum)
	for _, res := range results {
		a, ok := acc[res.Condition]
		if !ok {
			a = &accum{}
			acc[res.Condition] = a
		}
		a.total++
		if res.Success {
			a.solved++
		}
		a.promptTok += res.PromptTokensReal
		a.compTok += res.CompletionTokensReal
		a.totalTok += res.TotalTokensReal
		a.latency += res.Latency
	}

	summaries := make([]BenchmarkSummary, 0, len(acc))
	for cond, a := range acc {
		if a.total == 0 {
			continue
		}
		summaries = append(summaries, BenchmarkSummary{
			Condition:         cond,
			TotalProblems:     a.total,
			SolvedCount:       a.solved,
			Accuracy:          float64(a.solved) / float64(a.total),
			AvgPromptTokens:   float64(a.promptTok) / float64(a.total),
			AvgCompleteTokens: float64(a.compTok) / float64(a.total),
			AvgTotalTokens:    float64(a.totalTok) / float64(a.total),
			AvgLatencyMs:      float64(a.latency.Milliseconds()) / float64(a.total),
		})
	}
	return summaries
}

func extractXYZ(text string) (string, string, string) {
	reX := regexp.MustCompile(`(?i)x\s*=\s*(\d+)`)
	reY := regexp.MustCompile(`(?i)y\s*=\s*(\d+)`)
	reZ := regexp.MustCompile(`(?i)z\s*=\s*(\d+)`)

	mX := reX.FindStringSubmatch(text)
	mY := reY.FindStringSubmatch(text)
	mZ := reZ.FindStringSubmatch(text)

	var x, y, z string
	if len(mX) > 1 {
		x = mX[1]
	}
	if len(mY) > 1 {
		y = mY[1]
	}
	if len(mZ) > 1 {
		z = mZ[1]
	}
	return x, y, z
}
