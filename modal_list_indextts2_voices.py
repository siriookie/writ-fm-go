"""List files currently stored in the IndexTTS2 Modal voice volume.

Usage:
  modal run modal_list_indextts2_voices.py
"""

from __future__ import annotations

from pathlib import Path

import modal

VOICE_ROOT = "/voices"
VOICE_VOLUME = modal.Volume.from_name("writ-fm-indextts2-voices", create_if_missing=True)

app = modal.App("writ-fm-indextts2-voices-inspector")


@app.function(volumes={VOICE_ROOT: VOICE_VOLUME})
def list_voices():
    root = Path(VOICE_ROOT)
    if not root.exists():
        print(f"{VOICE_ROOT} does not exist")
        return

    entries = sorted(root.rglob("*"), key=lambda p: str(p).lower())
    files = [entry for entry in entries if entry.is_file()]
    if not entries:
        print(f"{VOICE_ROOT} is empty")
        return

    print(f"Files in {VOICE_ROOT}:")
    for entry in entries:
        kind = "dir" if entry.is_dir() else "file"
        size = entry.stat().st_size if entry.is_file() else 0
        rel = entry.relative_to(root)
        print(f"- {rel} [{kind}] {size} bytes")

    if not files:
        print("No files found under the mounted voice root.")


@app.local_entrypoint()
def main():
    list_voices.remote()
