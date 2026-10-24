package realeval

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// Runner manages live execution across all conditions with real Gemini API calls.
type Runner struct {
	Client     *GeminiClient
	KB         *KnowledgeBase
	ReportPath string
	RNG        *rand.Rand
}

// NewRunner initializes the runner with client, knowledge base, and CSR structures.
func NewRunner(client *GeminiClient, reportPath string) *Runner {
	return &Runner{
		Client:     client,
		KB:         NewKnowledgeBase(),
		ReportPath: reportPath,
		RNG:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RunSingleIntegrationTest tests end-to-end integration with a single problem.
func (r *Runner) RunSingleIntegrationTest(ctx context.Context) (*TrialResult, error) {
	if r.Client == nil {
		return nil, fmt.Errorf("client cannot be nil: missing Gemini API key")
	}

	testProb := EvalProblem{
		ID:     "INTEGRATION-01",
		Type:   ProblemConstruction,
		N:      big.NewInt(5),
		Prompt: "Find positive integers x, y, z satisfying 4/5 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.",
	}

	retrieval := r.KB.RetrieveMBVMCSR(testProb.N)
	sysPrompt := "You are an expert mathematician and theorem prover. Always justify your steps rigorously. For constructions, state x = <val>, y = <val>, z = <val>."
	userPrompt := fmt.Sprintf("%s\n\nContext:\n%s", testProb.Prompt, formatRules(retrieval.Rules))

	start := time.Now()
	resp, err := r.Client.GenerateContent(ctx, sysPrompt, userPrompt)
	latency := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("single test Gemini API call failed: %w", err)
	}

	var ansText string
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		ansText = resp.Candidates[0].Content.Parts[0].Text
	}

	success, details := VerifyResponse(testProb, ansText)

	result := &TrialResult{
		ProblemID:            testProb.ID,
		Condition:            ConditionMBVMCSR,
		ModelRequested:       r.Client.Model,
		PromptTokensReal:     resp.UsageMetadata.PromptTokenCount,
		CompletionTokensReal: resp.UsageMetadata.CandidatesTokenCount,
		TotalTokensReal:      resp.UsageMetadata.TotalTokenCount,
		ReasoningTokensReal:  resp.UsageMetadata.EffectiveReasoningTokens(),
		Latency:              latency,
		Success:              success,
		VerificationStatus:   details,
		ResponseText:         ansText,
		ModelVersion:         resp.ModelVersion,
		RetrievalPath:        retrieval.Path,
		RulesRetrieved:       retrieval.RuleIDs,
	}

	return result, nil
}

// RunBenchmarkWithProblems executes any slice of problems across the 4 conditions.
func (r *Runner) RunBenchmarkWithProblems(ctx context.Context, problems []EvalProblem) ([]TrialResult, []BenchmarkSummary, error) {
	allConditions := []ConditionType{
		ConditionDirect,
		ConditionFlatRAG,
		ConditionFilteredRAG,
		ConditionMBVMCSR,
	}

	results := make([]TrialResult, 0, len(problems)*4)

	for _, prob := range problems {
		conds := make([]ConditionType, len(allConditions))
		copy(conds, allConditions)
		r.RNG.Shuffle(len(conds), func(i, j int) { conds[i], conds[j] = conds[j], conds[i] })

		for _, cond := range conds {
			sysPrompt, userPrompt, ret := r.buildPrompt(cond, prob)

			start := time.Now()
			resp, callErr := r.Client.GenerateContent(ctx, sysPrompt, userPrompt)
			latency := time.Since(start)

			trial := TrialResult{
				ProblemID:      prob.ID,
				Condition:      cond,
				ModelRequested: r.Client.Model,
				Latency:        latency,
				RetrievalPath:  ret.Path,
				RulesRetrieved: ret.RuleIDs,
			}

			if callErr != nil {
				trial.ErrorMessage = callErr.Error()
				trial.VerificationStatus = "API call failed"
				results = append(results, trial)
				continue
			}

			var ansText string
			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				ansText = resp.Candidates[0].Content.Parts[0].Text
			}

			trial.PromptTokensReal = resp.UsageMetadata.PromptTokenCount
			trial.CompletionTokensReal = resp.UsageMetadata.CandidatesTokenCount
			trial.TotalTokensReal = resp.UsageMetadata.TotalTokenCount
			trial.ReasoningTokensReal = resp.UsageMetadata.EffectiveReasoningTokens()
			trial.ModelVersion = resp.ModelVersion
			trial.ResponseText = ansText

			success, details := VerifyResponse(prob, ansText)
			trial.Success = success
			trial.VerificationStatus = details

			results = append(results, trial)
		}
	}

	summaries := computeSummaries(results)

	if r.ReportPath != "" {
		payload := struct {
			Summaries []BenchmarkSummary `json:"summaries"`
			Results   []TrialResult      `json:"results"`
		}{
			Summaries: summaries,
			Results:   results,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return results, summaries, fmt.Errorf("failed to marshal benchmark report: %w", err)
		}
		if writeErr := os.WriteFile(r.ReportPath, data, 0644); writeErr != nil {
			return results, summaries, fmt.Errorf("failed to write report to %s: %w", r.ReportPath, writeErr)
		}
	}

	return results, summaries, nil
}

// RunFullBenchmark executes all 30 development benchmark problems across 4 conditions.
func (r *Runner) RunFullBenchmark(ctx context.Context) ([]TrialResult, []BenchmarkSummary, error) {
	problems, err := GenerateBenchmark30()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate 30-problem benchmark: %w", err)
	}
	return r.RunBenchmarkWithProblems(ctx, problems)
}

// RunUnseenBenchmark executes the 20 unexposed multi-lemma benchmark problems across 4 conditions.
func (r *Runner) RunUnseenBenchmark(ctx context.Context) ([]TrialResult, []BenchmarkSummary, error) {
	problems, err := GenerateUnseenBenchmark20()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate 20-problem unseen benchmark: %w", err)
	}
	return r.RunBenchmarkWithProblems(ctx, problems)
}

// RunPilotBenchmark executes a 4-problem pilot (1 per type) across 4 conditions (16 trials).
func (r *Runner) RunPilotBenchmark(ctx context.Context) ([]TrialResult, []BenchmarkSummary, error) {
	unseen, err := GenerateUnseenBenchmark20()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate pilot problems: %w", err)
	}
	pilotProbs := []EvalProblem{
		unseen[1],  // U20-02: Construction on prime 26881 (1 mod 840)
		unseen[10], // U20-11: Algebraic proof for 3k+2
		unseen[14], // U20-15: Refutation of uniqueness
		unseen[18], // U20-19: Modular obstruction 1 mod 24 vs -3 mod 3
	}
	return r.RunBenchmarkWithProblems(ctx, pilotProbs)
}

func (r *Runner) buildPrompt(cond ConditionType, prob EvalProblem) (string, string, RetrievalResult) {
	sysPrompt := "You are an expert mathematician and theorem prover. Always justify your steps rigorously. For constructions, state x = <val>, y = <val>, z = <val>."
	var ret RetrievalResult

	switch cond {
	case ConditionDirect:
		return sysPrompt, prob.Prompt, ret

	case ConditionFlatRAG:
		ret = r.KB.RetrieveFlat()
		userPrompt := fmt.Sprintf("%s\n\nReference Theorems:\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysPrompt, userPrompt, ret

	case ConditionFilteredRAG:
		ret = r.KB.RetrieveFiltered(prob.N)
		userPrompt := fmt.Sprintf("%s\n\nRelevant Theorem:\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysPrompt, userPrompt, ret

	case ConditionMBVMCSR:
		ret = r.KB.RetrieveMBVMCSR(prob.N)
		userPrompt := fmt.Sprintf("%s\n\nMBVM-DB Smart Sliced Lemma Graph:\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysPrompt, userPrompt, ret

	default:
		return sysPrompt, prob.Prompt, ret
	}
}

func formatRules(rules []*erdosstraus.Rule) string {
	if len(rules) == 0 {
		return "None."
	}
	var sb strings.Builder
	for i, r := range rules {
		sb.WriteString(fmt.Sprintf("[%d] %s (%s):\n    Premise: %s\n    Conclusion: %s\n",
			i+1, r.ID, r.BibSource, strings.Join(r.Hypotheses, "; "), r.Conclusion))
	}
	return sb.String()
}

func computeSummaries(results []TrialResult) []BenchmarkSummary {
	type accum struct {
		totalRequested               int
		completedResponses           int
		infrastructureErrors         int
		solved                       int
		promptTok, compTok, totalTok int
		latency                      time.Duration
	}
	acc := make(map[ConditionType]*accum)
	for _, res := range results {
		a, exists := acc[res.Condition]
		if !exists {
			a = &accum{}
			acc[res.Condition] = a
		}
		a.totalRequested++
		if res.ErrorMessage != "" {
			a.infrastructureErrors++
			continue
		}
		a.completedResponses++
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
		if a.totalRequested == 0 {
			continue
		}
		accRate := 0.0
		availRate := float64(a.completedResponses) / float64(a.totalRequested)
		avgPrompt := 0.0
		avgComp := 0.0
		avgTot := 0.0
		avgLat := 0.0

		if a.completedResponses > 0 {
			accRate = float64(a.solved) / float64(a.completedResponses)
			avgPrompt = float64(a.promptTok) / float64(a.completedResponses)
			avgComp = float64(a.compTok) / float64(a.completedResponses)
			avgTot = float64(a.totalTok) / float64(a.completedResponses)
			avgLat = float64(a.latency.Milliseconds()) / float64(a.completedResponses)
		}

		summaries = append(summaries, BenchmarkSummary{
			Condition:            cond,
			TotalProblems:        a.totalRequested,
			CompletedResponses:   a.completedResponses,
			InfrastructureErrors: a.infrastructureErrors,
			SolvedCount:          a.solved,
			Accuracy:             accRate,
			AvailabilityRate:     availRate,
			AvgPromptTokens:      avgPrompt,
			AvgCompleteTokens:    avgComp,
			AvgTotalTokens:       avgTot,
			AvgLatencyMs:         avgLat,
		})
	}
	return summaries
}
