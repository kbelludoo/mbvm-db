package realeval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	DefaultGeminiModel = "gemini-2.5-flash"
	GeminiAPIBaseURL   = "https://generativelanguage.googleapis.com/v1beta/models"
)

// GeminiClient manages live HTTP communications with the official Google GenAI API.
type GeminiClient struct {
	APIKey     string
	Model      string
	HTTPClient *http.Client
}

// NewGeminiClient instantiates a client discovering API key from environment.
func NewGeminiClient(model string) (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no API key found: GEMINI_API_KEY or GOOGLE_API_KEY must be set in environment: %w", ErrMissingAPIKey)
	}
	if model == "" {
		model = DefaultGeminiModel
	}

	return &GeminiClient{
		APIKey: apiKey,
		Model:  model,
		HTTPClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}, nil
}

// GenerateContent sends prompt to Gemini API and returns the full response including real usage metadata.
func (c *GeminiClient) GenerateContent(ctx context.Context, systemPrompt, userPrompt string) (*GeminiGenerateResponse, error) {
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", GeminiAPIBaseURL, c.Model, c.APIKey)

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]any{
					{"text": fmt.Sprintf("%s\n\nTask: %s", systemPrompt, userPrompt)},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":     0.1,
			"maxOutputTokens": 8192,
		},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http call to Gemini API failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		return nil, fmt.Errorf("gemini api returned status %d: %s (%s): %w",
			resp.StatusCode, errResp.Error.Message, errResp.Error.Status, ErrAPIError)
	}

	var genResp GeminiGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return nil, fmt.Errorf("failed to decode response JSON: %w", err)
	}

	return &genResp, nil
}

// ErrMissingAPIKey is returned when no API key is present in environment.
var ErrMissingAPIKey = fmt.Errorf("missing Gemini API key")

// ErrAPIError is returned when the remote API reports an HTTP or service error.
var ErrAPIError = fmt.Errorf("gemini api error")
