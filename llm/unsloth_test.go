package llm

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// the payload an unsloth studio actually answers /v1/models with.
const studioModelsPayload = `{
  "object": "list",
  "data": [
    {
      "id": "unsloth/Qwen3.8-27B-GGUF",
      "object": "model",
      "created": 1788022590,
      "owned_by": "unsloth-studio",
      "context_length": 13312,
      "max_context_length": 13312,
      "native_context_length": 262144,
      "loaded": true
    }
  ]
}`

func studioStub(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("asked for %s, want /v1/models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("authorization header = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	base, err := unslothBaseURL(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func TestFetchUnslothContextWindow(t *testing.T) {
	// the served window, not the model's native 262144
	base := studioStub(t, http.StatusOK, studioModelsPayload)
	got, err := fetchUnslothContextWindow(base, "test-key", "unsloth/Qwen3.8-27B-GGUF")
	if err != nil {
		t.Fatal(err)
	}
	if got != 13312 {
		t.Fatalf("context window = %d, want 13312", got)
	}
}

func TestFetchUnslothContextWindowFallsBackToMax(t *testing.T) {
	base := studioStub(t, http.StatusOK, `{"data":[{"id":"m","max_context_length":8192}]}`)
	got, err := fetchUnslothContextWindow(base, "test-key", "m")
	if err != nil {
		t.Fatal(err)
	}
	if got != 8192 {
		t.Fatalf("context window = %d, want 8192", got)
	}
}

func TestFetchUnslothContextWindowErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		model  string
	}{
		{"model not loaded", http.StatusOK, studioModelsPayload, "some/other-model"},
		{"no context reported", http.StatusOK, `{"data":[{"id":"m"}]}`, "m"},
		{"unauthenticated", http.StatusUnauthorized, `{"error":{"message":"Not authenticated"}}`, "m"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := studioStub(t, tc.status, tc.body)
			if _, err := fetchUnslothContextWindow(base, "test-key", tc.model); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestContextWindowOfMatchesOnPrefix(t *testing.T) {
	// providers hand back ids of varying specificity
	if got := ContextWindowOf(PROVIDER_OPENAI, "gpt-5.2"); got != 400000 {
		t.Fatalf("gpt-5.2 window = %d, want 400000", got)
	}
	if got := ContextWindowOf(PROVIDER_ANTHROPIC, "claude-sonnet-4-5-20250929"); got != 200000 {
		t.Fatalf("sonnet window = %d, want 200000", got)
	}
	if got := ContextWindowOf(PROVIDER_ANTHROPIC, ""); got != 0 {
		t.Fatalf("empty model window = %d, want 0", got)
	}
	if got := ContextWindowOf(PROVIDER_OPENAI, "some-model-we-do-not-know"); got != 0 {
		t.Fatalf("unknown model window = %d, want 0", got)
	}
}
