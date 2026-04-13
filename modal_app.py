"""Modal deployment for writ-fm music generation server.

Usage:
  # 1. First-time: download model weights into the persistent volume (~14 GB, ~10-30 min)
  modal run modal_app.py::preload_models

  # 2. Deploy the web endpoint
  modal deploy modal_app.py

  # 3. Use from Go CLI
  MUSIC_GEN_URL=https://<workspace>--writ-fm-music-gen-web.modal.run \
    go run ./cmd/musicgen/ --all --min 5
"""

import modal

APP_DIR = "/app/music-gen.server"
CHECKPOINTS_DIR = f"{APP_DIR}/checkpoints"

# Persistent volume — model weights downloaded once, reused across cold starts.
volume = modal.Volume.from_name("writ-fm-models", create_if_missing=True)


def _patch_acestep():
    """Fix None-safety bugs in ACE-Step metadata_utils.py."""
    import importlib.util

    spec = importlib.util.find_spec("acestep.core.generation.handler.metadata_utils")
    path = spec.origin
    with open(path) as f:
        src = f.read()
    src = src.replace(
        'key_scale if key_scale.strip() else "N/A"',
        '(key_scale.strip() or "N/A") if key_scale is not None else "N/A"',
    ).replace(
        'if time_signature.strip() and time_signature != "N/A" and time_signature:',
        'if time_signature and time_signature.strip() and time_signature != "N/A":',
    )
    with open(path, "w") as f:
        f.write(src)
    print(f"✅ Patched {path}")


# ---------------------------------------------------------------------------
# Container image — editable system install avoids venv/Modal typing_extensions clash
# ---------------------------------------------------------------------------
image = (
    modal.Image.debian_slim(python_version="3.12")
    .apt_install("git", "ffmpeg", "libsndfile1")
    .run_commands(
        f"git clone https://github.com/kortexa-ai/music-gen.server {APP_DIR}",
        "pip install uv",
        # Editable install into system Python (keeps source at APP_DIR so
        # _PROJECT_ROOT resolves to /app/music-gen.server for checkpoints).
        f"cd {APP_DIR} && uv pip install --system -e .",
    )
    .run_function(_patch_acestep)
    .env(
        {
            "ENABLE_LM": "0",
            "MPLBACKEND": "agg",
            "PYTORCH_CUDA_ALLOC_CONF": "expandable_segments:True",
            "MODEL_PRECISION": "float16",
        }
    )
)

app = modal.App("writ-fm-music-gen", image=image)


def _fix_path():
    """Move Modal's injected deps to end of sys.path.

    Modal prepends /__modal/deps/ which ships an old typing_extensions that
    lacks Sentinel, breaking pydantic-core >= 2.27.  Demoting it lets the
    system-installed version win.
    """
    import sys

    modal_deps = [p for p in sys.path if "/__modal/deps" in p]
    for p in modal_deps:
        sys.path.remove(p)
        sys.path.append(p)


# ---------------------------------------------------------------------------
# One-time model preload  (modal run modal_app.py::preload_models)
# ---------------------------------------------------------------------------
@app.function(
    gpu="T4",
    volumes={CHECKPOINTS_DIR: volume},
    timeout=3600,
)
def preload_models():
    """Download ACE-Step model weights into the persistent volume."""
    _fix_path()
    import os

    os.environ["ENABLE_LM"] = "0"
    from kortexa.music_gen.pipelines import pipeline_manager

    print("Downloading DiT model weights…")
    pipeline_manager.get_dit()
    volume.commit()
    print("✅ Models saved to volume — ready to deploy")


# ---------------------------------------------------------------------------
# Web endpoint  (modal deploy modal_app.py)
# ---------------------------------------------------------------------------
@app.function(
    gpu="T4",
    volumes={CHECKPOINTS_DIR: volume},
    timeout=600,
    scaledown_window=300,
)
@modal.asgi_app()
def web():
    _fix_path()
    import os

    os.environ["PRELOAD_MODELS"] = "1"
    from kortexa.music_gen.server import app as fastapi_app

    return fastapi_app
