package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIClientGenerate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         int
		response       any
		prompt         string
		model          string
		apiKey         string
		want           string
		wantAuth       string
		wantErr        error
		wantErrContain string
	}{
		{
			name:   "success",
			status: http.StatusOK,
			response: map[string]any{
				"choices": []map[string]any{
					{"message": map[string]any{"content": "hello from model"}},
				},
			},
			prompt:   "write me a script",
			model:    "gpt-test",
			apiKey:   "secret",
			want:     "hello from model",
			wantAuth: "Bearer secret",
		},
		{
			name:    "empty prompt",
			prompt:  "   ",
			model:   "gpt-test",
			wantErr: ErrEmptyResponse,
		},
		{
			name:           "missing model",
			prompt:         "hi",
			wantErrContain: "model is required",
		},
		{
			name:   "server side error",
			status: http.StatusBadRequest,
			response: map[string]any{
				"error": map[string]any{"message": "invalid request"},
			},
			prompt:         "hi",
			model:          "gpt-test",
			wantErrContain: "invalid request",
		},
		{
			name:   "empty choices",
			status: http.StatusOK,
			response: map[string]any{
				"choices": []map[string]any{},
			},
			prompt:  "hi",
			model:   "gpt-test",
			wantErr: ErrEmptyResponse,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string
			var gotAuth string
			var gotModel string
			var gotPrompt string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")

				var body chatCompletionsRequest
				_ = json.NewDecoder(r.Body).Decode(&body)
				gotModel = body.Model
				if len(body.Messages) > 0 {
					gotPrompt = body.Messages[0].Content
				}

				status := tt.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			client := NewOpenAIClient(srv.URL, tt.apiKey, tt.model)
			got, err := client.Generate(context.Background(), tt.prompt)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Generate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if tt.wantErrContain != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrContain) {
					t.Fatalf("Generate() error = %v, want substring %q", err, tt.wantErrContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("Generate() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Generate() = %q, want %q", got, tt.want)
			}
			if gotPath != "/v1/chat/completions" {
				t.Fatalf("path = %q, want /v1/chat/completions", gotPath)
			}
			if gotModel != tt.model {
				t.Fatalf("model = %q, want %q", gotModel, tt.model)
			}
			if gotPrompt != tt.prompt {
				t.Fatalf("prompt = %q, want %q", gotPrompt, tt.prompt)
			}
			if gotAuth != tt.wantAuth {
				t.Fatalf("authorization = %q, want %q", gotAuth, tt.wantAuth)
			}
		})
	}
}

func TestNormalizeOpenAIBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "root", in: "https://api.example.com", want: "https://api.example.com/v1"},
		{name: "already v1", in: "https://api.example.com/v1", want: "https://api.example.com/v1"},
		{name: "trailing slash", in: "https://api.example.com/v1/", want: "https://api.example.com/v1"},
		{name: "blank", in: " ", want: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeOpenAIBaseURL(tt.in); got != tt.want {
				t.Fatalf("normalizeOpenAIBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
