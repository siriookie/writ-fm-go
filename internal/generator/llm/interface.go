package llm

import (
	"context"
	"errors"
)

// ErrEmptyResponse is returned when a backend succeeds technically but
// produces no usable script text.
var ErrEmptyResponse = errors.New("generator/llm: empty response")

// Client is the consumer-side abstraction for text generation backends.
// All prompt-building and content-generation flows depend on this interface,
// not on a specific CLI or HTTP provider.
type Client interface {
	Generate(ctx context.Context, prompt string) (string, error)
}
