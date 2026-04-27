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

func TestGetHostVoice_Mimo25VoiceDesign(t *testing.T) {
	t.Parallel()

	voice, err := GetHostVoice("nyx", "mimo25_voicedesign")
	if err != nil {
		t.Fatalf("GetHostVoice() error = %v", err)
	}
	if voice != "af_heart" {
		t.Fatalf("voice = %q, want %q", voice, "af_heart")
	}
}

func TestSignalPersonaRequiresConcreteNewsExplanation(t *testing.T) {
	t.Parallel()

	host, err := GetHost("signal")
	if err != nil {
		t.Fatalf("GetHost() error = %v", err)
	}
	combined := host.Identity + host.VoiceStyle + host.Philosophy + host.AntiPatterns + host.PerformanceNotes
	for _, want := range []string{
		"先把事件按时间、地点、人物、动作讲顺",
		"各方说法的差异",
		"各方说法",
		"责任链",
		"普通人在这件事里真正承受的风险",
		"不要只谈「媒体如何报道」",
		"不要空谈时代、舆论、叙事、结构",
		"不要使用「把事实钉住」",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("signal persona missing %q:\n%s", want, combined)
		}
	}
}
