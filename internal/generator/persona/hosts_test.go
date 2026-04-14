package persona

import (
	"strings"
	"testing"
)

func TestGetHost(t *testing.T) {
	t.Parallel()

	host, err := GetHost("liminal_operator")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	if host.Name != "The Liminal Operator" {
		t.Fatalf("host.Name = %q, want %q", host.Name, "The Liminal Operator")
	}
}

func TestGetHost_Unknown(t *testing.T) {
	t.Parallel()

	_, err := GetHost("missing")
	if err == nil || !strings.Contains(err.Error(), "unknown host") {
		t.Fatalf("GetHost() error = %v, want unknown host error", err)
	}
}

func TestGetHostVoice(t *testing.T) {
	t.Parallel()

	voice, err := GetHostVoice("ember")
	if err != nil {
		t.Fatalf("GetHostVoice() error = %v", err)
	}
	if voice != "af_bella" {
		t.Fatalf("voice = %q, want %q", voice, "af_bella")
	}
}
