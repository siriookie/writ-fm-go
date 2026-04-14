package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultKokoroTimeout = 300 * time.Second

// KokoroTTS renders speech by shelling out to a Kokoro Python environment.
type KokoroTTS struct {
	kokoroDir      string
	timeout        time.Duration
	commandContext func(context.Context, string, ...string) *exec.Cmd
	tempDir        string
	goos           string
}

// NewKokoroTTS returns a Kokoro-backed TTS client rooted at kokoroDir.
func NewKokoroTTS(kokoroDir string) *KokoroTTS {
	return &KokoroTTS{
		kokoroDir:      strings.TrimSpace(kokoroDir),
		timeout:        defaultKokoroTimeout,
		commandContext: exec.CommandContext,
		goos:           runtime.GOOS,
	}
}

// Synthesize renders text to WAV bytes and writes them to dst.
func (k *KokoroTTS) Synthesize(ctx context.Context, text, voice string, dst io.Writer) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return ErrEmptyText
	}
	if dst == nil {
		return fmt.Errorf("generator/tts: destination writer is required")
	}

	python := pythonBinForGOOS(k.kokoroDir, k.goos)
	if _, err := os.Stat(python); err != nil {
		return fmt.Errorf("generator/tts: Kokoro python not found at %s: %w", python, err)
	}

	tmpFile, err := os.CreateTemp(k.tempDir, "kokoro-*.wav")
	if err != nil {
		return fmt.Errorf("generator/tts: create temp output: %w", err)
	}
	tmpPath := tmpFile.Name()
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("generator/tts: close temp output: %w", err)
	}
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	commandContext := k.commandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}

	cmd := commandContext(ctx, python, "-c", kokoroInlineScript(text, voice, tmpPath))
	cmd.Dir = k.kokoroDir
	cmd.Env = append(os.Environ(),
		"HF_HUB_OFFLINE=1",
		"TRANSFORMERS_OFFLINE=1",
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return context.DeadlineExceeded
		}

		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg != "" {
			return fmt.Errorf("generator/tts: Kokoro synthesis failed: %s: %w", msg, err)
		}
		return fmt.Errorf("generator/tts: Kokoro synthesis failed: %w", err)
	}

	audio, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("generator/tts: read temp output: %w", err)
	}
	if len(audio) == 0 {
		return fmt.Errorf("generator/tts: Kokoro produced empty audio")
	}
	if _, err := dst.Write(audio); err != nil {
		return fmt.Errorf("generator/tts: write output: %w", err)
	}
	return nil
}

func pythonBin(kokoroDir string) string {
	return pythonBinForGOOS(kokoroDir, runtime.GOOS)
}

func pythonBinForGOOS(kokoroDir, goos string) string {
	if goos == "windows" {
		return filepath.Join(kokoroDir, ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(kokoroDir, ".venv", "bin", "python")
}

func kokoroInlineScript(text, voice, outputPath string) string {
	return fmt.Sprintf(`
import os
os.environ["HF_HUB_OFFLINE"] = "1"
os.environ["TRANSFORMERS_OFFLINE"] = "1"
import warnings
warnings.filterwarnings("ignore")

from kokoro import KPipeline
import numpy as np
import soundfile as sf

pipe = KPipeline(lang_code=%q, repo_id="hexgrad/Kokoro-82M")
generator = pipe(%q, voice=%q, speed=1.0)
audio_segments = []
for _, _, audio in generator:
    audio_segments.append(audio)

if len(audio_segments) == 1:
    full_audio = audio_segments[0]
else:
    full_audio = np.concatenate(audio_segments)

sf.write(%q, full_audio, 24000)
print("SUCCESS")
`, detectKokoroLangCode(text), text, voice, outputPath)
}

func detectKokoroLangCode(text string) string {
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			return "z"
		case r >= 0x3400 && r <= 0x4DBF:
			return "z"
		case r >= 0x3000 && r <= 0x303F:
			return "z"
		}
	}
	return "a"
}
