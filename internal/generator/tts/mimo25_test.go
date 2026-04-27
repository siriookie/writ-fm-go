package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMimo25VoiceDesignSynthesize(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("api-key"); got != "key" {
			t.Fatalf("api-key = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("authorization = %q, want empty", got)
		}

		var raw map[string]any
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if raw["model"] != "mimo-v2.5-tts-voicedesign" {
			t.Fatalf("model = %q", raw["model"])
		}

		messages, ok := raw["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("messages = %#v", raw["messages"])
		}
		user := messages[0].(map[string]any)
		if user["role"] != "user" || !strings.Contains(user["content"].(string), "午夜电台男声") {
			t.Fatalf("user message = %#v", user)
		}
		assistant := messages[1].(map[string]any)
		if assistant["role"] != "assistant" || assistant["content"] != "hello world" {
			t.Fatalf("assistant message = %#v", assistant)
		}

		audio, ok := raw["audio"].(map[string]any)
		if !ok {
			t.Fatalf("audio = %#v", raw["audio"])
		}
		if audio["format"] != "wav" {
			t.Fatalf("format = %q", audio["format"])
		}
		if _, ok := audio["voice"]; ok {
			t.Fatalf("audio should not contain voice: %#v", audio)
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"audio": map[string]any{
							"data": base64.StdEncoding.EncodeToString([]byte("wav-bytes")),
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewMimo25VoiceDesignTTS("key", server.URL, "")
	var dst bytes.Buffer
	if err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q", got)
	}
}

func TestMimo25VoiceDesignPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		voice   string
		want    string
		wantErr string
	}{
		{
			name:  "known voice id",
			voice: "af_bella",
			want:  "温暖、自然",
		},
		{
			name:  "custom voice design",
			voice: "voice_design:一位清亮、聪明、略带戏谑的年轻女性声音。",
			want:  "清亮、聪明",
		},
		{
			name:    "empty custom voice design",
			voice:   "voice_design:   ",
			wantErr: "empty MiMo 2.5 voice design prompt",
		},
		{
			name:    "unknown voice id",
			voice:   "missing_voice",
			wantErr: "unknown MiMo 2.5 voice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mimo25VoiceDesignPrompt(tt.voice)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("mimo25VoiceDesignPrompt() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("mimo25VoiceDesignPrompt() error = %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Fatalf("prompt = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestMimo25SynthesizeRejectsUnknownVoice(t *testing.T) {
	t.Parallel()

	client := NewMimo25VoiceDesignTTS("key", "http://127.0.0.1:1", "")
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "unknown_voice", &dst)
	if err == nil || !strings.Contains(err.Error(), "unknown MiMo 2.5 voice") {
		t.Fatalf("Synthesize() error = %v, want unknown voice error", err)
	}
}
