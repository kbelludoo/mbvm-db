package realeval

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type testRoundTripFunc func(*http.Request) (*http.Response, error)

func (f testRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGeminiAPIKeySentViaHeader(t *testing.T) {
	const key = "fake-key-for-tests-only"
	called := false
	client := &GeminiClient{
		APIKey: key,
		Model:  "gemini-2.5-flash",
		HTTPClient: &http.Client{Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			called = true
			if req.URL.RawQuery != "" {
				t.Errorf("Gemini request URL unexpectedly has query parameters")
			}
			if strings.Contains(req.URL.String(), key) {
				t.Errorf("Gemini API key leaked into request URL")
			}
			if req.Header.Get("x-goog-api-key") != key {
				t.Errorf("missing API key in x-goog-api-key header")
			}
			if req.Header.Get("Content-Type") != "application/json" {
				t.Errorf("unexpected Content-Type header")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{
					"candidates":[{"content":{"parts":[{"text":"x = 1, y = 2, z = 3"}]}}],
					"usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":23,"thoughtsTokenCount":4,"totalTokenCount":44},
					"modelVersion":"gemini-test"
				}`)),
				Header: make(http.Header),
			}, nil
		})},
	}

	response, err := client.GenerateContent(context.Background(), "system", "task")
	if err != nil {
		t.Fatalf("Gemini mocked HTTP request failed: %v", err)
	}
	if !called {
		t.Fatal("mock HTTP transport was not invoked")
	}
	if response.ModelVersion != "gemini-test" {
		t.Errorf("model version: got %q, want gemini-test", response.ModelVersion)
	}
	if response.UsageMetadata.TotalTokenCount != 44 || response.UsageMetadata.EffectiveReasoningTokens() != 4 {
		t.Errorf("unexpected usage metadata: %+v", response.UsageMetadata)
	}
}
