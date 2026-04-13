package musicgen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGenerate_ReturnsDecodedAudio(t *testing.T) {
	want := []byte("fake-audio-data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/generate" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Caption == "" {
			t.Error("caption must not be empty")
		}
		encoded := base64.StdEncoding.EncodeToString(want)
		_ = json.NewEncoder(w).Encode(map[string]any{"audios": []string{encoded}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	audio, err := c.Generate(context.Background(), GenerateRequest{
		Caption:  "ambient drone",
		Duration: 30,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if string(audio) != string(want) {
		t.Errorf("audio = %q, want %q", audio, want)
	}
}

func TestGenerate_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Generate(context.Background(), GenerateRequest{Caption: "x", Duration: 30})
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestGenerate_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{"audios": []string{""}})
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	c := NewClient(srv.URL)
	_, err := c.Generate(ctx, GenerateRequest{Caption: "x", Duration: 30})
	if err == nil {
		t.Fatal("expected error on context cancel, got nil")
	}
}

func TestGenerate_EmptyAudiosArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"audios": []string{}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Generate(context.Background(), GenerateRequest{Caption: "x", Duration: 30})
	if err == nil {
		t.Fatal("expected error for empty audios, got nil")
	}
}

func TestGenerate_RequestFieldsEncoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req GenerateRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		if req.Caption != "jazz piano" {
			t.Errorf("caption = %q, want %q", req.Caption, "jazz piano")
		}
		if req.Duration != 120 {
			t.Errorf("duration = %v, want 120", req.Duration)
		}
		if req.AudioFormat != "flac" {
			t.Errorf("audio_format = %q, want flac", req.AudioFormat)
		}
		if !req.Instrumental {
			t.Error("instrumental must be true")
		}
		if req.InferenceSteps != 25 {
			t.Errorf("inference_steps = %d, want 25", req.InferenceSteps)
		}
		if !req.Thinking {
			t.Error("thinking must be true")
		}
		encoded := base64.StdEncoding.EncodeToString([]byte("ok"))
		_ = json.NewEncoder(w).Encode(map[string]any{"audios": []string{encoded}})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Generate(context.Background(), GenerateRequest{
		Caption:        "jazz piano",
		Duration:       120,
		AudioFormat:    "flac",
		Instrumental:   true,
		InferenceSteps: 25,
		Thinking:       true,
		Seed:           -1,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

func TestHealth_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Health(context.Background()); err != nil {
		t.Fatalf("Health: %v", err)
	}
}

func TestHealth_Down(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.Health(context.Background()); err == nil {
		t.Fatal("expected error when server returns 503")
	}
}
