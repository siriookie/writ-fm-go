package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIndexTTS2Synthesize(t *testing.T) {
	t.Parallel()

	fakeWAV := []byte("RIFF....WAVEfmt ")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req indexTTS2Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Text == "" || req.Voice == "" {
			http.Error(w, "missing fields", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fakeWAV)
	}))
	defer srv.Close()

	client := NewIndexTTS2TTS(srv.URL)
	var dst bytes.Buffer

	if err := client.Synthesize(context.Background(), "hello world", "liminal_operator", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if !bytes.Equal(dst.Bytes(), fakeWAV) {
		t.Fatalf("Synthesize() wrote %q, want %q", dst.Bytes(), fakeWAV)
	}
}

func TestIndexTTS2SynthesizeSendsJSONBody(t *testing.T) {
	t.Parallel()

	var gotReq indexTTS2Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("wav"))
	}))
	defer srv.Close()

	client := NewIndexTTS2TTS(srv.URL)
	var dst bytes.Buffer
	_ = client.Synthesize(context.Background(), "test text", "dr_resonance", &dst)

	if gotReq.Text != "test text" {
		t.Errorf("request text = %q, want %q", gotReq.Text, "test text")
	}
	if gotReq.Voice != "dr_resonance" {
		t.Errorf("request voice = %q, want %q", gotReq.Voice, "dr_resonance")
	}
}

func TestIndexTTS2SynthesizeEmptyText(t *testing.T) {
	t.Parallel()

	client := NewIndexTTS2TTS("http://localhost:9999")
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "  ", "liminal_operator", &dst)
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("Synthesize() error = %v, want ErrEmptyText", err)
	}
}

func TestIndexTTS2SynthesizeMissingEndpoint(t *testing.T) {
	t.Parallel()

	client := NewIndexTTS2TTS("")
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "liminal_operator", &dst)
	if err == nil || !strings.Contains(err.Error(), "endpoint is required") {
		t.Fatalf("Synthesize() error = %v, want endpoint required message", err)
	}
}

func TestIndexTTS2SynthesizeServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewIndexTTS2TTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "liminal_operator", &dst)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("Synthesize() error = %v, want 500 error", err)
	}
}

func TestIndexTTS2SynthesizeEmptyAudio(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewIndexTTS2TTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "liminal_operator", &dst)
	if err == nil || !strings.Contains(err.Error(), "empty audio") {
		t.Fatalf("Synthesize() error = %v, want empty audio error", err)
	}
}

func TestIndexTTS2SynthesizeContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		http.Error(w, "too late", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewIndexTTS2TTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(ctx, "hello", "liminal_operator", &dst)
	if err == nil {
		t.Fatal("Synthesize() expected error on cancelled context, got nil")
	}
}

func TestIndexTTS2SynthesizeNilWriter(t *testing.T) {
	t.Parallel()

	client := NewIndexTTS2TTS("http://localhost:9999")
	err := client.Synthesize(context.Background(), "hello", "liminal_operator", nil)
	if err == nil || !strings.Contains(err.Error(), "destination writer is required") {
		t.Fatalf("Synthesize() error = %v, want destination writer error", err)
	}
}

func TestIndexTTS2ImplementsClient(t *testing.T) {
	t.Parallel()
	var _ Client = (*IndexTTS2TTS)(nil)
}

func TestIndexTTS2URLTrailingSlash(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("wav"))
		_, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	client := NewIndexTTS2TTS(srv.URL + "/")
	var dst bytes.Buffer
	_ = client.Synthesize(context.Background(), "hello", "liminal_operator", &dst)

	if strings.HasSuffix(gotPath, "//") {
		t.Errorf("request path %q has double slash", gotPath)
	}
}
