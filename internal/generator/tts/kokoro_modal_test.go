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

func TestKokoroModalSynthesize(t *testing.T) {
	t.Parallel()

	fakeWAV := []byte("RIFF....WAVEfmt ")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req kokoroModalRequest
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

	client := NewKokoroModalTTS(srv.URL)
	var dst bytes.Buffer

	if err := client.Synthesize(context.Background(), "hello world", "am_michael", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if !bytes.Equal(dst.Bytes(), fakeWAV) {
		t.Fatalf("Synthesize() wrote %q, want %q", dst.Bytes(), fakeWAV)
	}
}

func TestKokoroModalSynthesize_SendsJSONBody(t *testing.T) {
	t.Parallel()

	var gotReq kokoroModalRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("wav"))
	}))
	defer srv.Close()

	client := NewKokoroModalTTS(srv.URL)
	var dst bytes.Buffer
	_ = client.Synthesize(context.Background(), "test text", "af_bella", &dst)

	if gotReq.Text != "test text" {
		t.Errorf("request text = %q, want %q", gotReq.Text, "test text")
	}
	if gotReq.Voice != "af_bella" {
		t.Errorf("request voice = %q, want %q", gotReq.Voice, "af_bella")
	}
}

func TestKokoroModalSynthesize_EmptyText(t *testing.T) {
	t.Parallel()

	client := NewKokoroModalTTS("http://localhost:9999")
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "  ", "am_michael", &dst)
	if !errors.Is(err, ErrEmptyText) {
		t.Fatalf("Synthesize() error = %v, want ErrEmptyText", err)
	}
}

func TestKokoroModalSynthesize_MissingEndpoint(t *testing.T) {
	t.Parallel()

	client := NewKokoroModalTTS("")
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "am_michael", &dst)
	if err == nil || !strings.Contains(err.Error(), "endpoint is required") {
		t.Fatalf("Synthesize() error = %v, want endpoint required message", err)
	}
}

func TestKokoroModalSynthesize_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := NewKokoroModalTTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "am_michael", &dst)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("Synthesize() error = %v, want 500 error", err)
	}
}

func TestKokoroModalSynthesize_EmptyAudio(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		// write nothing
	}))
	defer srv.Close()

	client := NewKokoroModalTTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(context.Background(), "hello", "am_michael", &dst)
	if err == nil || !strings.Contains(err.Error(), "empty audio") {
		t.Fatalf("Synthesize() error = %v, want empty audio error", err)
	}
}

func TestKokoroModalSynthesize_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// block long enough for the cancelled context to win
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
		http.Error(w, "too late", http.StatusInternalServerError)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	client := NewKokoroModalTTS(srv.URL)
	var dst bytes.Buffer
	err := client.Synthesize(ctx, "hello", "am_michael", &dst)
	if err == nil {
		t.Fatal("Synthesize() expected error on cancelled context, got nil")
	}
}

func TestKokoroModalSynthesize_NilWriter(t *testing.T) {
	t.Parallel()

	client := NewKokoroModalTTS("http://localhost:9999")
	err := client.Synthesize(context.Background(), "hello", "am_michael", nil)
	if err == nil || !strings.Contains(err.Error(), "destination writer is required") {
		t.Fatalf("Synthesize() error = %v, want destination writer error", err)
	}
}

// TestKokoroModalTTS_ImplementsClient ensures the type satisfies the Client interface at compile time.
func TestKokoroModalTTS_ImplementsClient(t *testing.T) {
	t.Parallel()
	var _ Client = (*KokoroModalTTS)(nil)
}

// TestKokoroModalSynthesize_URLTrailingSlash verifies that trailing slashes on the
// endpoint URL do not result in a double-slash in the request path.
func TestKokoroModalSynthesize_URLTrailingSlash(t *testing.T) {
	t.Parallel()

	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("wav"))
		_, _ = io.ReadAll(r.Body)
	}))
	defer srv.Close()

	client := NewKokoroModalTTS(srv.URL + "/")
	var dst bytes.Buffer
	_ = client.Synthesize(context.Background(), "hello", "am_michael", &dst)

	if strings.HasSuffix(gotPath, "//") {
		t.Errorf("request path %q has double slash", gotPath)
	}
}
