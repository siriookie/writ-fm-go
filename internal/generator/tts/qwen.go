package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultQwenTimeout = 60 * time.Second
	defaultQwenWSURL   = "wss://dashscope.aliyuncs.com/api-ws/v1/realtime"
	defaultQwenModel   = "qwen3-tts-flash-realtime"
)

type websocketDialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

// QwenTTS synthesizes speech through DashScope's realtime TTS websocket API.
type QwenTTS struct {
	apiKey               string
	model                string
	wsURL                string
	sampleRate           int
	format               string
	rate                 float32
	volume               int
	pitch                float32
	instructions         string
	optimizeInstructions bool
	timeout              time.Duration
	dialer               websocketDialer
}

// NewQwenTTS returns a Qwen realtime TTS adapter.
func NewQwenTTS(apiKey, model string) *QwenTTS {
	model = strings.TrimSpace(model)
	if model == "" {
		model = defaultQwenModel
	}
	return &QwenTTS{
		apiKey:     strings.TrimSpace(apiKey),
		model:      model,
		wsURL:      defaultQwenWSURL,
		sampleRate: 24000,
		format:     "wav",
		rate:       1.0,
		volume:     50,
		pitch:      1.0,
		timeout:    defaultQwenTimeout,
		dialer:     websocket.DefaultDialer,
	}
}

// Synthesize renders text via a single websocket session and writes the audio to dst.
func (q *QwenTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if q.apiKey == "" {
		return fmt.Errorf("generator/tts: Qwen API key is required")
	}

	ctx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()

	header := make(http.Header)
	header.Set("Authorization", "bearer "+q.apiKey)
	header.Set("OpenAI-Beta", "realtime=v1")

	conn, _, err := q.dialer.DialContext(ctx, q.wsURL+"?model="+url.QueryEscape(q.model), header)
	if err != nil {
		return fmt.Errorf("generator/tts: Qwen dial failed: %w", err)
	}
	defer conn.Close()
	go closeOnContext(ctx, conn)

	if err := waitForQwenEvent(ctx, conn, "session.created", nil); err != nil {
		return err
	}
	if err := q.sendSessionUpdate(conn, voice); err != nil {
		return err
	}
	if err := waitForQwenEvent(ctx, conn, "session.updated", nil); err != nil {
		return err
	}
	if err := writeJSON(conn, map[string]any{
		"event_id": nextEventID(),
		"type":     "input_text_buffer.append",
		"text":     text,
	}); err != nil {
		return fmt.Errorf("generator/tts: Qwen append failed: %w", err)
	}
	if err := writeJSON(conn, map[string]any{
		"event_id": nextEventID(),
		"type":     "input_text_buffer.commit",
	}); err != nil {
		return fmt.Errorf("generator/tts: Qwen commit failed: %w", err)
	}

	var audio bytes.Buffer
	for {
		event, err := readQwenEvent(ctx, conn)
		if err != nil {
			return err
		}
		switch event.Type {
		case "response.audio.delta":
			chunk, err := base64.StdEncoding.DecodeString(event.Delta)
			if err != nil {
				return fmt.Errorf("generator/tts: decode Qwen audio chunk: %w", err)
			}
			audio.Write(chunk)
		case "response.audio.done":
			if audio.Len() == 0 {
				return fmt.Errorf("generator/tts: Qwen returned empty audio")
			}
			if _, err := io.Copy(dst, &audio); err != nil {
				return fmt.Errorf("generator/tts: write output: %w", err)
			}
			return nil
		case "error":
			return fmt.Errorf("generator/tts: Qwen error: %s", event.Error.Message)
		}
	}
}

func (q *QwenTTS) sendSessionUpdate(conn *websocket.Conn, voice string) error {
	payload := map[string]any{
		"event_id": nextEventID(),
		"type":     "session.update",
		"session": map[string]any{
			"mode":            "server_commit",
			"voice":           voice,
			"response_format": q.format,
			"sample_rate":     q.sampleRate,
			"speech_rate":     q.rate,
			"volume":          q.volume,
			"pitch_rate":      q.pitch,
		},
	}
	if q.instructions != "" {
		session := payload["session"].(map[string]any)
		session["instructions"] = q.instructions
		session["optimize_instructions"] = q.optimizeInstructions
	}
	if err := writeJSON(conn, payload); err != nil {
		return fmt.Errorf("generator/tts: Qwen session.update failed: %w", err)
	}
	return nil
}

type qwenServerEvent struct {
	Type  string `json:"type"`
	Delta string `json:"delta,omitempty"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func readQwenEvent(ctx context.Context, conn *websocket.Conn) (qwenServerEvent, error) {
	var event qwenServerEvent
	_, data, err := conn.ReadMessage()
	if err != nil {
		select {
		case <-ctx.Done():
			return event, ctx.Err()
		default:
			return event, fmt.Errorf("generator/tts: Qwen read failed: %w", err)
		}
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return event, fmt.Errorf("generator/tts: decode Qwen event: %w", err)
	}
	return event, nil
}

func waitForQwenEvent(ctx context.Context, conn *websocket.Conn, want string, sink *bytes.Buffer) error {
	for {
		event, err := readQwenEvent(ctx, conn)
		if err != nil {
			return err
		}
		if event.Type == want {
			return nil
		}
		if event.Type == "response.audio.delta" && sink != nil {
			chunk, err := base64.StdEncoding.DecodeString(event.Delta)
			if err != nil {
				return fmt.Errorf("generator/tts: decode Qwen audio chunk: %w", err)
			}
			sink.Write(chunk)
		}
		if event.Type == "error" && event.Error != nil {
			return fmt.Errorf("generator/tts: Qwen error: %s", event.Error.Message)
		}
	}
}
