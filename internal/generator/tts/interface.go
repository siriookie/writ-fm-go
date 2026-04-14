package tts

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrEmptyText is returned when the caller asks TTS to synthesize blank text.
	ErrEmptyText = errors.New("generator/tts: empty text")
)

// Client is the consumer-side TTS interface used by renderer/generator flows.
type Client interface {
	Synthesize(ctx context.Context, text, voice string, dst io.Writer) error
}
