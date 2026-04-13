package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	maxMessages    = 100
	messageCooldown = 5 * time.Minute
)

// storedMessage is the on-disk and in-memory representation of a listener message.
type storedMessage struct {
	Message   string    `json:"message"`
	IP        string    `json:"ip"`
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
}

// publicMessage is returned by GET /messages (IP hidden).
type publicMessage struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Read      bool      `json:"read"`
}

// messageStore manages listener messages in memory, persisted to a JSON file.
// It also enforces per-IP rate limiting.
type messageStore struct {
	path string

	mu       sync.Mutex
	messages []storedMessage
	cooldown map[string]time.Time // IP → last message time
}

func newMessageStore(path string) *messageStore {
	ms := &messageStore{
		path:     path,
		cooldown: make(map[string]time.Time),
	}
	if path != "" {
		ms.load()
	}
	return ms
}

// RateLimited reports whether the given IP is still within the cooldown window.
// If not rate-limited, it records the new message time and returns false.
func (ms *messageStore) RateLimited(ip string) (waitSeconds int, limited bool) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if last, ok := ms.cooldown[ip]; ok {
		remaining := messageCooldown - time.Since(last)
		if remaining > 0 {
			return int(remaining.Seconds()) + 1, true
		}
	}
	ms.cooldown[ip] = time.Now()
	return 0, false
}

// Add saves a new message from the given IP.
func (ms *messageStore) Add(text, ip string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.messages = append(ms.messages, storedMessage{
		Message:   text,
		IP:        ip,
		Timestamp: time.Now(),
	})
	// Keep only the last maxMessages entries.
	if len(ms.messages) > maxMessages {
		ms.messages = ms.messages[len(ms.messages)-maxMessages:]
	}
	ms.save()
}

// Recent returns the most recent n messages with IP addresses removed.
func (ms *messageStore) Recent(n int) []publicMessage {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	src := ms.messages
	if n < len(src) {
		src = src[len(src)-n:]
	}
	out := make([]publicMessage, len(src))
	for i, m := range src {
		out[len(src)-1-i] = publicMessage{ // newest first
			Message:   m.Message,
			Timestamp: m.Timestamp,
			Read:      m.Read,
		}
	}
	return out
}

// load reads stored messages from disk. Errors are silently ignored so that a
// missing or corrupt file never prevents the server from starting.
func (ms *messageStore) load() {
	data, err := os.ReadFile(ms.path)
	if err != nil {
		return
	}
	var msgs []storedMessage
	if err := json.Unmarshal(data, &msgs); err != nil {
		return
	}
	ms.messages = msgs
}

// save writes the current messages to disk atomically. Called while mu is held.
func (ms *messageStore) save() {
	if ms.path == "" {
		return
	}
	data, err := json.MarshalIndent(ms.messages, "", "  ")
	if err != nil {
		return
	}
	dir := filepath.Dir(ms.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	tmp := ms.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, ms.path)
}
