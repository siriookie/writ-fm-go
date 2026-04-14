package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultCosyVoiceTimeout = 60 * time.Second
	defaultCosyVoiceWSURL   = "wss://dashscope.aliyuncs.com/api-ws/v1/inference/"
)

// CosyVoiceTTS synthesizes speech through DashScope's CosyVoice websocket API.
type CosyVoiceTTS struct {
	apiKey     string
	wsURL      string
	sampleRate int
	format     string
	rate       float32
	volume     int
	textType   string
	timeout    time.Duration
	dialer     websocketDialer
}

// NewCosyVoiceTTS returns a CosyVoice adapter with sane defaults.
func NewCosyVoiceTTS(apiKey string) *CosyVoiceTTS {
	return &CosyVoiceTTS{
		apiKey:     strings.TrimSpace(apiKey),
		wsURL:      defaultCosyVoiceWSURL,
		sampleRate: 24000,
		format:     "wav",
		rate:       1.0,
		volume:     50,
		textType:   "PlainText",
		timeout:    defaultCosyVoiceTimeout,
		dialer:     websocket.DefaultDialer,
	}
}

// Synthesize renders text via a single websocket task and writes the audio to dst.
func (c *CosyVoiceTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}
	if c.apiKey == "" {
		return fmt.Errorf("generator/tts: CosyVoice API key is required")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	header := make(http.Header)
	header.Set("Authorization", "bearer "+c.apiKey)
	header.Set("X-DashScope-DataInspection", "enable")

	conn, _, err := c.dialer.DialContext(ctx, c.wsURL, header)
	if err != nil {
		return fmt.Errorf("generator/tts: CosyVoice dial failed: %w", err)
	}
	defer conn.Close()
	go closeOnContext(ctx, conn)

	taskID := nextEventID()
	if err := writeJSON(conn, map[string]any{
		"header": map[string]any{
			"action":    "run-task",
			"task_id":   taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"task_group": "audio",
			"task":       "tts",
			"function":   "SpeechSynthesizer",
			"model":      "cosyvoice-v1",
			"parameters": map[string]any{
				"text_type":   c.textType,
				"voice":       voice,
				"format":      c.format,
				"sample_rate": c.sampleRate,
				"volume":      c.volume,
				"rate":        c.rate,
				"pitch":       1,
			},
			"input": map[string]any{},
		},
	}); err != nil {
		return fmt.Errorf("generator/tts: CosyVoice run-task failed: %w", err)
	}
	if err := waitForCosyEvent(ctx, conn, "task-started"); err != nil {
		return err
	}
	if err := writeJSON(conn, map[string]any{
		"header": map[string]any{
			"action":    "continue-task",
			"task_id":   taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"input": map[string]any{"text": text},
		},
	}); err != nil {
		return fmt.Errorf("generator/tts: CosyVoice continue-task failed: %w", err)
	}
	if err := writeJSON(conn, map[string]any{
		"header": map[string]any{
			"action":    "finish-task",
			"task_id":   taskID,
			"streaming": "duplex",
		},
		"payload": map[string]any{
			"input": map[string]any{},
		},
	}); err != nil {
		return fmt.Errorf("generator/tts: CosyVoice finish-task failed: %w", err)
	}

	var audio bytes.Buffer
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("generator/tts: CosyVoice read failed: %w", err)
			}
		}
		if msgType == websocket.BinaryMessage {
			audio.Write(data)
			continue
		}

		var event cosyVoiceEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("generator/tts: decode CosyVoice event: %w", err)
		}
		switch event.Header.Event {
		case "task-finished":
			if audio.Len() == 0 {
				return fmt.Errorf("generator/tts: CosyVoice returned empty audio")
			}
			if _, err := io.Copy(dst, &audio); err != nil {
				return fmt.Errorf("generator/tts: write output: %w", err)
			}
			return nil
		case "task-failed":
			return fmt.Errorf("generator/tts: CosyVoice error: %s", event.Header.ErrorMessage)
		}
	}
}

type cosyVoiceEvent struct {
	Header struct {
		Event        string `json:"event"`
		ErrorMessage string `json:"error_message,omitempty"`
	} `json:"header"`
}

func waitForCosyEvent(ctx context.Context, conn *websocket.Conn, want string) error {
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return fmt.Errorf("generator/tts: CosyVoice read failed: %w", err)
			}
		}
		var event cosyVoiceEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("generator/tts: decode CosyVoice event: %w", err)
		}
		if event.Header.Event == want {
			return nil
		}
		if event.Header.Event == "task-failed" {
			return fmt.Errorf("generator/tts: CosyVoice error: %s", event.Header.ErrorMessage)
		}
	}
}
