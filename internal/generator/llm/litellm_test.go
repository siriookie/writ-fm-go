package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestLiteLLMClientGenerate(t *testing.T) {
	t.Parallel()

	metadata := map[string]any{
		"generation_name": "test-generation",
		"trace_user_id":   "user-1",
		"tags":            []string{"api-test", "development"},
	}

	var gotPath string
	var gotAuth string
	var gotModel string
	var gotPrompt string
	var gotMetadata map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")

		var body liteLLMRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotModel = body.Model
		if len(body.Messages) > 0 {
			gotPrompt = body.Messages[0].Content
		}
		gotMetadata = body.Metadata

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": "hello from litellm"}},
			},
		})
	}))
	defer srv.Close()

	client := NewLiteLLMClient(srv.URL, "proxy-key", "gpt-3.5-turbo", metadata)
	got, err := client.Generate(context.Background(), "what llm are you")
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if got != "hello from litellm" {
		t.Fatalf("Generate() = %q, want %q", got, "hello from litellm")
	}
	if gotPath != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer proxy-key" {
		t.Fatalf("authorization = %q, want %q", gotAuth, "Bearer proxy-key")
	}
	if gotModel != "gpt-3.5-turbo" {
		t.Fatalf("model = %q, want %q", gotModel, "gpt-3.5-turbo")
	}
	if gotPrompt != "what llm are you" {
		t.Fatalf("prompt = %q, want %q", gotPrompt, "what llm are you")
	}
	if !reflect.DeepEqual(gotMetadata["generation_name"], metadata["generation_name"]) {
		t.Fatalf("generation_name = %v, want %v", gotMetadata["generation_name"], metadata["generation_name"])
	}
	if !reflect.DeepEqual(gotMetadata["trace_user_id"], metadata["trace_user_id"]) {
		t.Fatalf("trace_user_id = %v, want %v", gotMetadata["trace_user_id"], metadata["trace_user_id"])
	}
}

func TestLiteLLMClientGenerate_ErrorCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		client         *LiteLLMClient
		prompt         string
		serverStatus   int
		serverResponse any
		wantErr        error
		wantErrContain string
	}{
		{
			name:    "empty prompt",
			client:  NewLiteLLMClient("http://localhost:4000", "", "gpt-test", nil),
			prompt:  "   ",
			wantErr: ErrEmptyResponse,
		},
		{
			name:           "missing model",
			client:         NewLiteLLMClient("http://localhost:4000", "", "", nil),
			prompt:         "hello",
			wantErrContain: "model is required",
		},
		{
			name:           "missing base url",
			client:         NewLiteLLMClient("", "", "gpt-test", nil),
			prompt:         "hello",
			wantErrContain: "base URL is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := tt.client.Generate(context.Background(), tt.prompt)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Generate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Fatalf("Generate() error = %v, want substring %q", err, tt.wantErrContain)
			}
		})
	}
}

func TestLiteLLMClientGenerate_ServerErrorAndEmptyChoices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         int
		response       any
		wantErr        error
		wantErrContain string
	}{
		{
			name:   "server error",
			status: http.StatusBadRequest,
			response: map[string]any{
				"error": map[string]any{"message": "invalid request"},
			},
			wantErrContain: "invalid request",
		},
		{
			name:   "empty choices",
			status: http.StatusOK,
			response: map[string]any{
				"choices": []map[string]any{},
			},
			wantErr: ErrEmptyResponse,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			client := NewLiteLLMClient(srv.URL, "", "gpt-test", nil)
			_, err := client.Generate(context.Background(), "hello")
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Generate() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErrContain) {
				t.Fatalf("Generate() error = %v, want substring %q", err, tt.wantErrContain)
			}
		})
	}
}
