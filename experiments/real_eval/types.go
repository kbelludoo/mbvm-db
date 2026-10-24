package realeval

import (
	"math/big"
	"time"
)

// ConditionType labels the inference condition for the LLM.
type ConditionType string

const (
	ConditionDirectBaseline   ConditionType = "Gemini Direct (No Memory)"
	ConditionConventionalRAG ConditionType = "Gemini + Flat RAG (Linear Rule List)"
	ConditionMBVMSliced       ConditionType = "Gemini + MBVM-DB (Smart Slicing)"
)

// MathChallenge represents an unseen problem with ground-truth verification requirements.
type MathChallenge struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	N           *big.Int `json:"n"`
	Modulus     int64    `json:"modulus"`
	Residue     int64    `json:"residue"`
	TargetType  string   `json:"target_type"` // "solve_fractions", "prove_subfamily", "refute_claim"
}

// GeminiUsageMetadata captures the exact token counts reported by the Gemini API.
type GeminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
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
	Error         *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// TrialResult records the empirical metrics for a single LLM execution.
type TrialResult struct {
	ChallengeID          string        `json:"challenge_id"`
	Condition            ConditionType `json:"condition"`
	PromptTokensReal     int           `json:"prompt_tokens_real"`
	CompletionTokensReal int           `json:"completion_tokens_real"`
	TotalTokensReal      int           `json:"total_tokens_real"`
	Latency              time.Duration `json:"latency"`
	Success              bool          `json:"success"`
	ExtractedX           string        `json:"extracted_x,omitempty"`
	ExtractedY           string        `json:"extracted_y,omitempty"`
	ExtractedZ           string        `json:"extracted_z,omitempty"`
	ErrorMessage         string        `json:"error_message,omitempty"`
	RawResponseSample    string        `json:"raw_response_sample,omitempty"`
}
