package tts

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestCosyVoiceSynthesize(t *testing.T) {
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

		var msg map[string]any
		_ = conn.ReadJSON(&msg)
		if got := msg["header"].(map[string]any)["action"]; got != "run-task" {
			t.Fatalf("first action = %v", got)
		}
		_ = conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-started"}})

		_ = conn.ReadJSON(&msg)
		if got := msg["header"].(map[string]any)["action"]; got != "continue-task" {
			t.Fatalf("continue action = %v", got)
		}
		_ = conn.ReadJSON(&msg)
		if got := msg["header"].(map[string]any)["action"]; got != "finish-task" {
			t.Fatalf("finish action = %v", got)
		}

		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("wav-"))
		_ = conn.WriteMessage(websocket.BinaryMessage, []byte("bytes"))
		_ = conn.WriteJSON(map[string]any{"header": map[string]any{"event": "task-finished"}})
	}))
	defer server.Close()

	client := NewCosyVoiceTTS("key")
	client.wsURL = "ws" + strings.TrimPrefix(server.URL, "http")

	var dst bytes.Buffer
	if err := client.Synthesize(context.Background(), "hello", "longhua", &dst); err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := dst.String(); got != "wav-bytes" {
		t.Fatalf("Synthesize() wrote %q", got)
	}
}
