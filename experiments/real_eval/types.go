package realeval

import (
	"time"
)

// ConditionType labels the inference condition for the LLM.
type ConditionType string

const (
	ConditionDirectBaseline   ConditionType = "Condition 1: Gemini Direct (No Memory)"
	ConditionFlatRAG          ConditionType = "Condition 2: Flat RAG (Unfiltered Dump)"
	ConditionFilteredRAG      ConditionType = "Condition 3: Filtered Inverted Index (Conventional Relevant)"
	ConditionMBVMCSR          ConditionType = "Condition 4: MBVM-DB CSR Subgraph (Smart Slicing)"
)

// GeminiUsageMetadata captures the exact token counts reported by the Gemini API.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
	CandidatesTokenDetails struct {
		ReasoningTokenCount int `json:"reasoningTokenCount,omitempty"`
	} `json:"candidatesTokenDetails,omitempty"`
}

// GeminiGenerateResponse models the JSON payload returned by Google GenAI API.
type GeminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata GeminiUsageMetadata `json:"usageMetadata"`
	ModelVersion  string              `json:"modelVersion,omitempty"`
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// TrialResult records empirical metrics for a single LLM execution.
type TrialResult struct {
	ProblemID            string        `json:"problem_id"`
	Condition            ConditionType `json:"condition"`
	PromptTokensReal     int           `json:"prompt_tokens_real"`
	CompletionTokensReal int           `json:"completion_tokens_real"`
	TotalTokensReal      int           `json:"total_tokens_real"`
	ReasoningTokensReal  int           `json:"reasoning_tokens_real,omitempty"`
	Latency              time.Duration `json:"latency"`
	Success              bool          `json:"success"`
	VerificationStatus   string        `json:"verification_status"`
	RetrievalPath        string        `json:"retrieval_path,omitempty"`
	RulesRetrieved       []string      `json:"rules_retrieved,omitempty"`
	ModelVersion         string        `json:"model_version,omitempty"`
	ExtractedX           string        `json:"extracted_x,omitempty"`
	ExtractedY           string        `json:"extracted_y,omitempty"`
	ExtractedZ           string        `json:"extracted_z,omitempty"`
	ErrorMessage         string        `json:"error_message,omitempty"`
	FullResponse         string        `json:"full_response,omitempty"`
}

// BenchmarkSummary holds aggregate metrics across all 4 conditions.
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
