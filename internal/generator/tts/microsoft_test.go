package tts

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMicrosoftSynthesize(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Ocp-Apim-Subscription-Key"); got != "key" {
			t.Fatalf("subscription key = %q", got)
		}
		if got := r.Header.Get("X-Microsoft-OutputFormat"); got != defaultMicrosoftOutputFormat {
			t.Fatalf("output format = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if !strings.Contains(string(body), "zh-CN-YunxiNeural") {
			t.Fatalf("SSML missing mapped voice: %s", string(body))
		}
		if !strings.Contains(string(body), `xml:lang="zh-CN"`) {
			t.Fatalf("SSML missing zh-CN xml:lang: %s", string(body))
		}
		if !strings.Contains(string(body), "hello &amp; goodbye") {
			t.Fatalf("SSML missing escaped text: %s", string(body))
		}
		_, _ = w.Write([]byte("wav-bytes"))
	}))
	defer server.Close()

	client := NewMicrosoftTTS("key", "eastus")
	client.baseURL = server.URL

	var dst bytes.Buffer
	if err := client.Synthesize(context.Background(), "hello & goodbye", "am_michael", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q", got)
	}
}

func TestMapMicrosoftVoice(t *testing.T) {
	t.Parallel()

	if got := mapMicrosoftVoice("af_bella"); got != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("mapped voice = %q", got)
	}
	if got := mapMicrosoftVoice("zh-CN-XiaoxiaoNeural"); got != "zh-CN-XiaoxiaoNeural" {
		t.Fatalf("passthrough voice = %q", got)
	}
}

func TestMicrosoftVoiceLang(t *testing.T) {
	t.Parallel()

	if got := microsoftVoiceLang("zh-CN-XiaoxiaoNeural"); got != "zh-CN" {
		t.Fatalf("microsoftVoiceLang() = %q, want zh-CN", got)
	}
}
