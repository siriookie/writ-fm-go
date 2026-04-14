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

	"github.com/gorilla/websocket"
)

func TestQwenSynthesize(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "bearer key" {
			t.Fatalf("authorization = %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Upgrade() error = %v", err)
		}
		defer conn.Close()

		_ = conn.WriteJSON(map[string]any{
			"type":    "session.created",
			"session": map[string]any{"id": "sess-1"},
		})

		var msg map[string]any
		_ = conn.ReadJSON(&msg)
		if got := msg["type"]; got != "session.update" {
			t.Fatalf("first client message type = %v", got)
		}
		_ = conn.WriteJSON(map[string]any{"type": "session.updated"})

		_ = conn.ReadJSON(&msg)
		if got := msg["type"]; got != "input_text_buffer.append" {
			t.Fatalf("append message type = %v", got)
		}
		_ = conn.ReadJSON(&msg)
		if got := msg["type"]; got != "input_text_buffer.commit" {
			t.Fatalf("commit message type = %v", got)
		}

		_ = conn.WriteJSON(map[string]any{"type": "response.created"})
		_ = conn.WriteJSON(map[string]any{"type": "response.audio.delta", "delta": base64.StdEncoding.EncodeToString([]byte("wav-"))})
		_ = conn.WriteJSON(map[string]any{"type": "response.audio.delta", "delta": base64.StdEncoding.EncodeToString([]byte("bytes"))})
		_ = conn.WriteJSON(map[string]any{"type": "response.audio.done"})
	}))
	defer server.Close()

	client := NewQwenTTS("key", "")
	client.wsURL = "ws" + strings.TrimPrefix(server.URL, "http")

	var dst bytes.Buffer
	if err := client.Synthesize(context.Background(), "hello", "Cherry", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q", got)
	}
}

func TestReadQwenEvent_DecodeError(t *testing.T) {
	t.Parallel()

	var event qwenServerEvent
	if err := json.Unmarshal([]byte(`{"type":1}`), &event); err == nil {
		t.Fatal("expected unmarshal type mismatch")
	}
}
