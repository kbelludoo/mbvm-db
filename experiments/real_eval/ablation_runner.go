package realeval

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// AblationTrialResult captures data for the 4-group N=26881 ablation study.
type AblationTrialResult struct {
	TrialIndex           int           `json:"trial_index"`
	Condition            ConditionType `json:"condition"`
	ModelRequested       string        `json:"model_requested"`
	ModelVersion         string        `json:"model_version"`
	PromptTokensReal     int           `json:"prompt_tokens_real"`
	CompletionTokensReal int           `json:"completion_tokens_real"`
	TotalTokensReal      int           `json:"total_tokens_real"`
	ReasoningTokensReal  int           `json:"reasoning_tokens_real"`
	Latency              time.Duration `json:"latency"`
	Success              bool          `json:"success"`
	VerificationStatus   string        `json:"verification_status"`
	ResponseText         string        `json:"response_text,omitempty"`
	RetrievalPath        string        `json:"retrieval_path"`
	RulesRetrieved       []string      `json:"rules_retrieved"`
	ErrorMessage         string        `json:"error_message,omitempty"`
}

// AblationSummary compiles comparative metrics across the ablation conditions.
type AblationSummary struct {
	Condition            ConditionType `json:"condition"`
	TrialsCount          int           `json:"trials_count"`
	CompletedResponses   int           `json:"completed_responses"`
	InfrastructureErrors int           `json:"infrastructure_errors"`
	SolvedCount          int           `json:"solved_count"`
	MathFailuresCount    int           `json:"math_failures_count"`
	MathematicalAccuracy float64       `json:"mathematical_accuracy"`
	AvailabilityRate     float64       `json:"availability_rate"`
	AvgPromptTokens      float64       `json:"avg_prompt_tokens"`
	AvgCompleteTokens    float64       `json:"avg_complete_tokens"`
	AvgTotalTokens       float64       `json:"avg_total_tokens"`
	AvgReasoningTok      float64       `json:"avg_reasoning_tokens"`
	AvgLatencyMs         float64       `json:"avg_latency_ms"`
}

// RunAblation26881 executes the 4-group controlled ablation on N=26881.
func (r *Runner) RunAblation26881(ctx context.Context, trialsPerCond int, outputPath string) ([]AblationTrialResult, []AblationSummary, error) {
	if r.Client == nil {
		return nil, nil, fmt.Errorf("client cannot be nil: missing Gemini API key")
	}

	n26881 := big.NewInt(26881)
	prob := EvalProblem{
		ID:     "U20-02-ABLATION",
		Type:   ProblemConstruction,
		N:      n26881,
		Prompt: "Find positive integers x, y, z satisfying 4/26881 = 1/x + 1/y + 1/z. State: x = <val>, y = <val>, z = <val>.",
	}

	conditions := []ConditionType{
		ConditionAblationDirect,
		ConditionAblationFormulaOnly,
		ConditionAblationFormulaRecipe,
		ConditionAblationCSRRecipe,
	}

	results := make([]AblationTrialResult, 0, len(conditions)*trialsPerCond)
	globalTrialIdx := 0

	for trialNum := 1; trialNum <= trialsPerCond; trialNum++ {
		// Randomize condition order per round to prevent temporal/caching bias
		roundConds := make([]ConditionType, len(conditions))
		copy(roundConds, conditions)
		r.RNG.Shuffle(len(roundConds), func(i, j int) { roundConds[i], roundConds[j] = roundConds[j], roundConds[i] })

		for _, cond := range roundConds {
			globalTrialIdx++
			sysPrompt, userPrompt, ret := r.buildAblationPrompt(cond, prob)

			start := time.Now()
			resp, callErr := r.Client.GenerateContent(ctx, sysPrompt, userPrompt)
			latency := time.Since(start)

			res := AblationTrialResult{
				TrialIndex:     globalTrialIdx,
				Condition:      cond,
				ModelRequested: r.Client.Model,
				Latency:        latency,
				RetrievalPath:  ret.Path,
				RulesRetrieved: ret.RuleIDs,
			}

			if callErr != nil {
				res.ErrorMessage = callErr.Error()
				res.VerificationStatus = "API call failed"
				results = append(results, res)
				continue
			}

			var ansText string
			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				ansText = resp.Candidates[0].Content.Parts[0].Text
			}

			res.PromptTokensReal = resp.UsageMetadata.PromptTokenCount
			res.CompletionTokensReal = resp.UsageMetadata.CandidatesTokenCount
			res.TotalTokensReal = resp.UsageMetadata.TotalTokenCount
			res.ReasoningTokensReal = resp.UsageMetadata.EffectiveReasoningTokens()
			res.ModelVersion = resp.ModelVersion
			res.ResponseText = ansText

			success, details := VerifyResponse(prob, ansText)
			res.Success = success
			res.VerificationStatus = details

			results = append(results, res)
		}
	}

	summaries := computeAblationSummaries(results)

	if outputPath != "" {
		payload := struct {
			TargetN   string                `json:"target_n"`
			Timestamp string                `json:"timestamp"`
			Summaries []AblationSummary     `json:"summaries"`
			Results   []AblationTrialResult `json:"results"`
		}{
			TargetN:   "26881 (prime = 1 mod 840, = 8401 mod 9240)",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Summaries: summaries,
			Results:   results,
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return results, summaries, fmt.Errorf("failed to marshal ablation report: %w", err)
		}
		if writeErr := os.WriteFile(outputPath, data, 0644); writeErr != nil {
			return results, summaries, fmt.Errorf("failed to write ablation report to %s: %w", outputPath, writeErr)
		}
	}

	return results, summaries, nil
}

func (r *Runner) buildAblationPrompt(cond ConditionType, prob EvalProblem) (string, string, RetrievalResult) {
	sysPrompt := "You are an expert mathematician and theorem prover. Always justify your steps rigorously. For constructions, state x = <val>, y = <val>, z = <val>."
	var ret RetrievalResult

	switch cond {
	case ConditionAblationDirect:
		return sysPrompt, prob.Prompt, ret

	case ConditionAblationFormulaOnly:
		ret = r.KB.RetrieveFiltered(prob.N)
		userPrompt := fmt.Sprintf("%s\n\nRelevant Theorem:\n%s", prob.Prompt, formatRules(ret.Rules))
		return sysPrompt, userPrompt, ret

	case ConditionAblationFormulaRecipe:
		ret = r.KB.RetrieveFiltered(prob.N)
		var sb strings.Builder
		sb.WriteString(prob.Prompt)
		sb.WriteString("\n\nRelevant Theorem:\n")
		sb.WriteString(formatRules(ret.Rules))
		sb.WriteString(buildRecipeText(ret.Rules, prob.N))
		return sysPrompt, sb.String(), ret

	case ConditionAblationCSRRecipe:
		ret = r.KB.RetrieveMBVMCSR(prob.N)
		var sb strings.Builder
		sb.WriteString(prob.Prompt)
		sb.WriteString("\n\nMBVM-DB Smart Sliced Lemma Graph:\n")
		sb.WriteString(formatRules(ret.Rules))
		sb.WriteString(buildRecipeText(ret.Rules, prob.N))
		return sysPrompt, sb.String(), ret

	default:
		return sysPrompt, prob.Prompt, ret
	}
}

func buildRecipeText(rules []*erdosstraus.Rule, n *big.Int) string {
	if n == nil || len(rules) == 0 {
		return ""
	}
	for _, r := range rules {
		if r.Covers(n) {
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("\nTopological Instantiation Recipe for n = %s:\n", n.String()))
			sb.WriteString(fmt.Sprintf("  • Active Root Lemma: %s (%s)\n", r.ID, r.BibSource))
			sb.WriteString(fmt.Sprintf("  • Substitution Formula: %s\n", r.Conclusion))
			return sb.String()
		}
	}
	return ""
}

func computeAblationSummaries(results []AblationTrialResult) []AblationSummary {
	type accum struct {
		totalRequested               int
		completedResponses           int
		infrastructureErrors         int
		solved                       int
		mathFailures                 int
		promptTok, compTok, totalTok int
		reasoningTok                 int
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
		} else {
			a.mathFailures++
		}
		a.promptTok += res.PromptTokensReal
		a.compTok += res.CompletionTokensReal
		a.totalTok += res.TotalTokensReal
		a.reasoningTok += res.ReasoningTokensReal
		a.latency += res.Latency
	}

	summaries := make([]AblationSummary, 0, len(acc))
	for cond, a := range acc {
		if a.totalRequested == 0 {
			continue
		}
		mathAcc := 0.0
		availRate := float64(a.completedResponses) / float64(a.totalRequested)
		avgPrompt := 0.0
		avgComp := 0.0
		avgTot := 0.0
		avgReasoning := 0.0
		avgLat := 0.0

		if a.completedResponses > 0 {
			mathAcc = float64(a.solved) / float64(a.completedResponses)
			avgPrompt = float64(a.promptTok) / float64(a.completedResponses)
			avgComp = float64(a.compTok) / float64(a.completedResponses)
			avgTot = float64(a.totalTok) / float64(a.completedResponses)
			avgReasoning = float64(a.reasoningTok) / float64(a.completedResponses)
			avgLat = float64(a.latency.Milliseconds()) / float64(a.completedResponses)
		}

		summaries = append(summaries, AblationSummary{
			Condition:            cond,
			TrialsCount:          a.totalRequested,
			CompletedResponses:   a.completedResponses,
			InfrastructureErrors: a.infrastructureErrors,
			SolvedCount:          a.solved,
			MathFailuresCount:    a.mathFailures,
			MathematicalAccuracy: mathAcc,
			AvailabilityRate:     availRate,
			AvgPromptTokens:      avgPrompt,
			AvgCompleteTokens:    avgComp,
			AvgTotalTokens:       avgTot,
			AvgReasoningTok:      avgReasoning,
			AvgLatencyMs:         avgLat,
		})
	}
	return summaries
}
