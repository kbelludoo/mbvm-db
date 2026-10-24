package realeval

import (
	"time"
)

// ConditionType labels the inference condition for the LLM.
type ConditionType string

const (
	ConditionDirectBaseline ConditionType = "Condition 1: Gemini Direct (No Memory)"
	ConditionDirect                       = ConditionDirectBaseline
	ConditionFlatRAG        ConditionType = "Condition 2: Flat RAG (Unfiltered Dump)"
	ConditionFilteredRAG    ConditionType = "Condition 3: Filtered Inverted Index (Conventional Relevant)"
	ConditionMBVMCSR        ConditionType = "Condition 4: MBVM-DB CSR Subgraph (Smart Slicing)"
)

// GeminiUsageMetadata captures the exact token counts reported by the Gemini API.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	ThoughtsTokenCount   int `json:"thoughtsTokenCount,omitempty"`
	CandidatesTokenDetails struct {
		ReasoningTokenCount int `json:"reasoningTokenCount,omitempty"`
	} `json:"candidatesTokenDetails,omitempty"`
}

// EffectiveReasoningTokens returns whichever field the Gemini API populated for reasoning.
func (u *GeminiUsageMetadata) EffectiveReasoningTokens() int {
	if u.ThoughtsTokenCount > 0 {
		return u.ThoughtsTokenCount
	}
	return u.CandidatesTokenDetails.ReasoningTokenCount
}

// GeminiGenerateResponse models the JSON payload returned by Google GenAI API.
type GeminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
	ModelVersion  string              `json:"modelVersion"`
}

// TrialResult records the metrics and verification of an individual problem attempt.
type TrialResult struct {
	ProblemID            string        `json:"problem_id"`
	Condition            ConditionType `json:"condition"`
	PromptTokensReal     int           `json:"prompt_tokens_real"`
	CompletionTokensReal int           `json:"completion_tokens_real"`
	TotalTokensReal      int           `json:"total_tokens_real"`
	ReasoningTokensReal  int           `json:"reasoning_tokens_real"`
	Latency              time.Duration `json:"latency"`
	Success              bool          `json:"success"`
	VerificationStatus   string        `json:"verification_status"`
	ResponseText         string        `json:"response_text,omitempty"`
	ModelVersion         string        `json:"model_version"`
	RetrievalPath        string        `json:"retrieval_path"`
	RulesRetrieved       []string      `json:"rules_retrieved"`
	ErrorMessage         string        `json:"error_message,omitempty"`
}

// BenchmarkSummary compiles aggregate performance statistics for a condition group.
type BenchmarkSummary struct {
	Condition         ConditionType `json:"condition"`
	TotalProblems     int           `json:"total_problems"`
	SolvedCount       int           `json:"solved_count"`
	Accuracy          float64       `json:"accuracy"`
	AvgPromptTokens   float64       `json:"avg_prompt_tokens"`
	AvgCompleteTokens float64       `json:"avg_complete_tokens"`
	AvgTotalTokens    float64       `json:"avg_total_tokens"`
	AvgLatencyMs      float64       `json:"avg_latency_ms"`
}
