package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMimoSynthesize(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer key" {
			t.Fatalf("authorization = %q", got)
		}

		var req mimoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.Model != "mimo-v2-tts" {
			t.Fatalf("model = %q", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "assistant" || req.Messages[0].Content != "hello world" {
			t.Fatalf("messages = %#v", req.Messages)
		}
		if req.Audio.Voice != "mimo_default" {
			t.Fatalf("voice = %q", req.Audio.Voice)
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

	client := NewMimoTTS("key", server.URL, "")
	var dst bytes.Buffer
	if err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q", got)
	}
}

func TestMapMimoVoice(t *testing.T) {
	t.Parallel()

	if got := mapMimoVoice("hello", "default_zh"); got != "default_zh" {
		t.Fatalf("passthrough voice = %q", got)
	}
	if got := mapMimoVoice("你好世界", "am_michael"); got != "mimo_default" {
		t.Fatalf("chinese text voice = %q", got)
	}
	if got := mapMimoVoice("hello", "zh-CN-XiaoxiaoNeural"); got != "default_zh" {
		t.Fatalf("zh voice = %q", got)
	}
	if got := mapMimoVoice("hello", "af_bella"); got != "default_zh" {
		t.Fatalf("alias voice = %q", got)
	}
	if got := mapMimoVoice("hello", "unknown_voice"); got != "default_en" {
		t.Fatalf("fallback voice = %q", got)
	}
}

func TestMapMimoVoiceKnownAliases(t *testing.T) {
	t.Parallel()

	if got := mapMimoVoice("中文内容", "bm_daniel"); got != "mimo_default" {
		t.Fatalf("male alias voice = %q, want mimo_default", got)
	}
	if got := mapMimoVoice("中文内容", "af_bella"); got != "default_zh" {
		t.Fatalf("female alias voice = %q, want default_zh", got)
	}
	if got := mapMimoVoice("中文内容", "zh_yunxi"); got != "mimo_default" {
		t.Fatalf("azure male alias voice = %q, want mimo_default", got)
	}
	if got := mapMimoVoice("中文内容", "zh_xiaoxiao"); got != "default_zh" {
		t.Fatalf("azure female alias voice = %q, want default_zh", got)
	}
}
