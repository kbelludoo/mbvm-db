package realeval

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"

	erdosstraus "github.com/kbelludoo/mbvm-db/experiments/erdos_straus"
)

// RunLiveBenchmark executes unseen problems against the real Gemini API across the 3 conditions.
func RunLiveBenchmark(ctx context.Context, client *GeminiClient) ([]TrialResult, error) {
	if client == nil {
		return nil, fmt.Errorf("gemini client cannot be nil: %w", ErrMissingAPIKey)
	}

	dataset := UnseenEvaluationDataset()
	results := make([]TrialResult, 0, len(dataset)*3)

	conditions := []ConditionType{
		ConditionDirectBaseline,
		ConditionConventionalRAG,
		ConditionMBVMSliced,
	}

	for _, challenge := range dataset {
		for _, condition := range conditions {
			sysPrompt, userPrompt := buildPrompts(condition, challenge)

			start := time.Now()
			resp, err := client.GenerateContent(ctx, sysPrompt, userPrompt)
			latency := time.Since(start)

			trial := TrialResult{
				ChallengeID: challenge.ID,
				Condition:   condition,
				Latency:     latency,
			}

			if err != nil {
				trial.ErrorMessage = err.Error()
				results = append(results, trial)
				continue
			}

			trial.PromptTokensReal = resp.UsageMetadata.PromptTokenCount
			trial.CompletionTokensReal = resp.UsageMetadata.CandidatesTokenCount
			trial.TotalTokensReal = resp.UsageMetadata.TotalTokenCount

			if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
				ansText := resp.Candidates[0].Content.Parts[0].Text
				trial.RawResponseSample = truncateSample(ansText, 250)

				if challenge.TargetType == "refute_claim" {
					trial.Success = strings.Contains(strings.ToLower(ansText), "impossível") ||
						strings.Contains(strings.ToLower(ansText), "impossible") ||
						strings.Contains(strings.ToLower(ansText), "refut")
				} else {
					x, y, z := extractXYZ(ansText)
					trial.ExtractedX = x
					trial.ExtractedY = y
					trial.ExtractedZ = z

					if x != "" && y != "" && z != "" {
						xBig, okX := new(big.Int).SetString(x, 10)
						yBig, okY := new(big.Int).SetString(y, 10)
						zBig, okZ := new(big.Int).SetString(z, 10)
						if okX && okY && okZ {
							sol := &erdosstraus.Solution{X: xBig, Y: yBig, Z: zBig}
							if vErr := erdosstraus.VerifySolution(challenge.N, sol); vErr == nil {
								trial.Success = true
							} else {
								trial.ErrorMessage = vErr.Error()
							}
						}
					}
				}
			}

			results = append(results, trial)
		}
	}

	// Persist real evaluation report
	raw, _ := json.MarshalIndent(results, "", "  ")
	_ = os.WriteFile("experiments/real_eval/real_eval_results.json", raw, 0644)

	return results, nil
}

func buildPrompts(condition ConditionType, challenge MathChallenge) (string, string) {
	sysBase := "You are a mathematical theorem prover. Solve or refute the given problem rigorously. " +
		"When unit fraction denominators are requested, clearly state: x = <val>, y = <val>, z = <val>."

	switch condition {
	case ConditionDirectBaseline:
		return sysBase, challenge.Description

	case ConditionConventionalRAG:
		rules := erdosstraus.StandardSeedRules()
		var sb strings.Builder
		sb.WriteString("Reference Knowledge Base (All Known Rules):\n")
		for _, r := range rules {
			sb.WriteString(fmt.Sprintf("- [%s] %s: Hypotheses=%v, Conclusion=%s\n", r.ID, r.Name, r.Hypotheses, r.Conclusion))
		}
		return sysBase + "\n\n" + sb.String(), challenge.Description

	case ConditionMBVMSliced:
		// MBVM Smart Slicing: Only retrieve relevant sub-family matching n mod M
		rules := erdosstraus.StandardSeedRules()
		var matched *erdosstraus.Rule
		for _, r := range rules {
			if r.Covers(challenge.N) {
				matched = r
				break
			}
		}
		var sb strings.Builder
		sb.WriteString("MBVM-DB Sliced Active Subgraph:\n")
		if matched != nil {
			sb.WriteString(fmt.Sprintf("- [%s] %s: Hypotheses=%v, Proof=%s\n", matched.ID, matched.Name, matched.Hypotheses, matched.ProofText))
		} else {
			sb.WriteString("- [RULE-MORDELL-DIVISOR] Universal algebraic decomposition: (4x-n)yz = nx(y+z)\n")
		}
		return sysBase + "\n\n" + sb.String(), challenge.Description

	default:
		return sysBase, challenge.Description
	}
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

func truncateSample(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
