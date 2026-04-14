"""Modal deployment for writ-fm Kokoro TTS service.

Usage:
  modal deploy modal_kokoro.py

The endpoint accepts:
  POST {"text": "...", "voice": "..."}

and returns raw WAV bytes.
"""

import io

import modal
from fastapi import Response

HF_CACHE = "/root/.cache/huggingface"

volume = modal.Volume.from_name("writ-fm-kokoro-models", create_if_missing=True)

image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("libsndfile1", "ffmpeg")
    .pip_install(
        "kokoro>=0.9.4",
        "soundfile",
        "numpy",
        "fastapi",
    )
    .env(
        {
            "HF_HUB_OFFLINE": "0",
            "TRANSFORMERS_OFFLINE": "0",
        }
    )
)

app = modal.App("writ-fm-kokoro", image=image)


def detect_lang_code(text: str) -> str:
    for ch in text:
        code = ord(ch)
        if 0x4E00 <= code <= 0x9FFF:
            return "z"
        if 0x3400 <= code <= 0x4DBF:
            return "z"
        if 0x3000 <= code <= 0x303F:
            return "z"
    return "a"


@app.function(
    volumes={HF_CACHE: volume},
    timeout=600,
)
def preload_models():
    """Download Kokoro-82M weights into the persistent volume."""
    from kokoro import KPipeline

    print("Downloading Kokoro-82M weights...")
    KPipeline(lang_code="a", repo_id="hexgrad/Kokoro-82M")
    KPipeline(lang_code="z", repo_id="hexgrad/Kokoro-82M")
    volume.commit()
    print("Kokoro-82M saved to volume and ready to deploy")


@app.cls(
    volumes={HF_CACHE: volume},
    timeout=120,
    scaledown_window=300,
)
class KokoroService:
    @modal.enter()
    def load_model(self):
        import os

        os.environ["HF_HUB_OFFLINE"] = "1"
        os.environ["TRANSFORMERS_OFFLINE"] = "1"

        from kokoro import KPipeline

        print("Loading Kokoro-82M pipelines...")
        self.pipes = {
            "a": KPipeline(lang_code="a", repo_id="hexgrad/Kokoro-82M"),
            "z": KPipeline(lang_code="z", repo_id="hexgrad/Kokoro-82M"),
        }
        print("Kokoro-82M ready")

    @modal.method()
    def synthesize(self, text: str, voice: str) -> bytes:
        """Render text to WAV bytes. Returns raw WAV (24 kHz, mono, PCM 16-bit)."""
        import numpy as np
        import soundfile as sf

        pipe = self.pipes[detect_lang_code(text)]
        segments = [audio for _, _, audio in pipe(text, voice=voice, speed=1.0)]
        if not segments:
            raise ValueError("Kokoro produced no audio segments")

        full = np.concatenate(segments) if len(segments) > 1 else segments[0]

        buf = io.BytesIO()
        sf.write(buf, full, samplerate=24000, format="WAV", subtype="PCM_16")
        return buf.getvalue()


@app.function()
@modal.fastapi_endpoint(method="POST", label="writ-fm-kokoro-synthesize")
def synthesize(body: dict) -> Response:
    text = (body.get("text") or "").strip()
    voice = (body.get("voice") or "am_michael").strip()

    if not text:
        return Response(content=b"text is required", status_code=422)
    if not voice:
        voice = "am_michael"

    wav_bytes = KokoroService().synthesize.remote(text, voice)
    return Response(content=wav_bytes, media_type="audio/wav")
