"""Modal deployment for WRIT-FM IndexTTS2 service.

Usage:
  # 1. Preload model weights into the persistent volume
  modal run modal_indextts2.py::preload_models

  # 2. Optionally upload your reference voices into the voice volume
  #    under /voices, for example:
  #    /voices/liminal_operator.wav
  #    /voices/dr_resonance.wav
  #    /voices/nyx.wav
  #    /voices/signal.wav
  #    /voices/ember.wav
  #
  #    You may also add /voices/voice_map.json to override the built-in mapping.
  #
  # 3. Deploy the HTTP endpoint
  modal deploy modal_indextts2.py

The endpoint accepts:
  POST {
    "text": "...",
    "voice": "liminal_operator",
    "emo_text": "",
    "use_emo_text": false,
    "emo_alpha": 0.6,
    "use_random": false
  }

and returns raw WAV bytes.
"""

from __future__ import annotations

import io
import json
import os
import sys
import tempfile
from pathlib import Path

import modal
from fastapi import Response

APP_DIR = "/app/index-tts"
CHECKPOINTS_DIR = "/checkpoints"
VOICE_ROOT = "/voices"

CHECKPOINTS_VOLUME = modal.Volume.from_name("writ-fm-indextts2-models", create_if_missing=True)
VOICE_VOLUME = modal.Volume.from_name("writ-fm-indextts2-voices", create_if_missing=True)

DEFAULT_VOICE_MAP = {
    # Your local sample ids.
    "dsm": "dsm.wav",
    "jcole": "jcole.wav",
    "mll": "mll.wav",
    "xiran": "xiran.wav",
    # Persona-oriented ids.
    "liminal_operator": "dsm.wav",
    "dr_resonance": "dsm.wav",
    "nyx": "mll.wav",
    "signal": "xiran.wav",
    "ember": "mll.wav",
    # Existing WRIT-FM schedule / backend aliases.
    "am_michael": "dsm.wav",
    "bm_daniel": "dsm.wav",
    "af_heart": "mll.wav",
    "am_onyx": "xiran.wav",
    "af_bella": "mll.wav",
    "zh_yunxi": "dsm.wav",
    "zh_xiaoxiao": "xiran.wav",
    "mimo_default": "dsm.wav",
    "default_zh": "mll.wav",
}

image = (
    modal.Image.debian_slim(python_version="3.11")
    .apt_install("git", "git-lfs", "ffmpeg", "libsndfile1", "curl", "unzip")
    .run_commands(
        "git lfs install --system",
        (
            f"mkdir -p /app && "
            "curl -L https://github.com/index-tts/index-tts/archive/refs/heads/main.zip "
            "-o /tmp/index-tts.zip && "
            "unzip -q /tmp/index-tts.zip -d /tmp && "
            f"mv /tmp/index-tts-main {APP_DIR} && "
            "rm -f /tmp/index-tts.zip"
        ),
        "pip install uv",
        "uv pip install --system 'fastapi[standard]'",
        (
            f"cd {APP_DIR} && "
            "uv pip install --system -e ."
        ),
    )
    .env(
        {
            "PYTHONPATH": APP_DIR,
            "HF_HUB_CACHE": f"{CHECKPOINTS_DIR}/hf_cache",
            "HF_HOME": f"{CHECKPOINTS_DIR}/hf_home",
        }
    )
)

app = modal.App("writ-fm-indextts2", image=image)


def _voice_map_path() -> Path:
    return Path(VOICE_ROOT) / "voice_map.json"


def load_voice_map() -> dict[str, str]:
    path = _voice_map_path()
    mapping = dict(DEFAULT_VOICE_MAP)
    if path.exists():
        with path.open("r", encoding="utf-8") as f:
            loaded = json.load(f)
        if isinstance(loaded, dict):
            for key, value in loaded.items():
                if isinstance(key, str) and isinstance(value, str) and key.strip() and value.strip():
                    mapping[key.strip()] = value.strip()
    return mapping


def resolve_voice_prompt(voice: str) -> str:
    voice = (voice or "").strip()
    if not voice:
        voice = "liminal_operator"

    mapping = load_voice_map()
    relative = mapping.get(voice, voice)
    path = Path(relative)
    if not path.is_absolute():
        path = Path(VOICE_ROOT) / relative
    if not path.exists():
        raise FileNotFoundError(
            f"reference voice prompt not found for {voice!r}: {path}. "
            f"Upload a wav file into the {VOICE_VOLUME.object_id if hasattr(VOICE_VOLUME, 'object_id') else 'voice'} volume "
            "or provide /voices/voice_map.json"
        )
    return str(path)


@app.function(
    volumes={CHECKPOINTS_DIR: CHECKPOINTS_VOLUME},
    timeout=3600,
)
def preload_models():
    from huggingface_hub import snapshot_download

    snapshot_download(
        repo_id="IndexTeam/IndexTTS-2",
        local_dir=CHECKPOINTS_DIR,
        local_dir_use_symlinks=False,
    )
    CHECKPOINTS_VOLUME.commit()
    print("IndexTTS2 checkpoints downloaded and committed")


@app.cls(
    gpu="L40S",
    volumes={
        CHECKPOINTS_DIR: CHECKPOINTS_VOLUME,
        VOICE_ROOT: VOICE_VOLUME,
    },
    timeout=1200,
    scaledown_window=300,
)
class IndexTTS2Service:
    @modal.enter()
    def load_model(self):
        sys.path.insert(0, APP_DIR)
        from indextts.infer_v2 import IndexTTS2

        self.tts = IndexTTS2(
            cfg_path=f"{CHECKPOINTS_DIR}/config.yaml",
            model_dir=CHECKPOINTS_DIR,
            use_fp16=True,
            use_cuda_kernel=True,
            use_deepspeed=False,
        )

    @modal.method()
    def synthesize(
        self,
        text: str,
        voice: str,
        emo_text: str = "",
        use_emo_text: bool = False,
        emo_alpha: float = 0.6,
        use_random: bool = False,
    ) -> bytes:
        text = (text or "").strip()
        if not text:
            raise ValueError("text is required")

        spk_audio_prompt = resolve_voice_prompt(voice)

        with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
            output_path = tmp.name

        try:
            kwargs = {
                "spk_audio_prompt": spk_audio_prompt,
                "text": text,
                "output_path": output_path,
                "verbose": False,
                "use_random": bool(use_random),
            }
            emo_text = (emo_text or "").strip()
            if use_emo_text:
                kwargs["use_emo_text"] = True
                kwargs["emo_alpha"] = float(emo_alpha)
                if emo_text:
                    kwargs["emo_text"] = emo_text

            self.tts.infer(**kwargs)
            with open(output_path, "rb") as f:
                return f.read()
        finally:
            try:
                os.remove(output_path)
            except FileNotFoundError:
                pass


@app.function()
@modal.fastapi_endpoint(method="POST", label="writ-fm-indextts2-synthesize")
def synthesize(body: dict) -> Response:
    text = (body.get("text") or "").strip()
    voice = (body.get("voice") or "liminal_operator").strip()
    emo_text = (body.get("emo_text") or "").strip()
    use_emo_text = bool(body.get("use_emo_text", False))
    emo_alpha = float(body.get("emo_alpha", 0.6))
    use_random = bool(body.get("use_random", False))

    if not text:
        return Response(content=b"text is required", status_code=422)

    wav_bytes = IndexTTS2Service().synthesize.remote(
        text=text,
        voice=voice,
        emo_text=emo_text,
        use_emo_text=use_emo_text,
        emo_alpha=emo_alpha,
        use_random=use_random,
    )
    return Response(content=wav_bytes, media_type="audio/wav")
