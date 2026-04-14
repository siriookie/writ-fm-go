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
	if host.Name != "临界" {
		t.Fatalf("host.Name = %q, want %q", host.Name, "临界")
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

	voice, err := GetHostVoice("ember", "kokoro")
	if err != nil {
		t.Fatalf("GetHostVoice() error = %v", err)
	}
	if voice != "zf_xiaobei" {
		t.Fatalf("voice = %q, want %q", voice, "zf_xiaobei")
	}
}

func TestGetHostVoice_BackendSpecific(t *testing.T) {
	t.Parallel()

	voice, err := GetHostVoice("signal", "microsoft")
	if err != nil {
		t.Fatalf("GetHostVoice() error = %v", err)
	}
	if voice != "zh_yunxi" {
		t.Fatalf("voice = %q, want %q", voice, "zh_yunxi")
	}
}
