"""Upload local IndexTTS2 reference voices into the Modal volume.

Usage:
  python modal_sync_indextts2_voices.py
  python modal_sync_indextts2_voices.py --source ./local_voices
  python modal_sync_indextts2_voices.py --source ./local_voices --dry-run

The script uploads:
  - *.wav / *.mp3 / *.flac / *.m4a reference voice files
  - voice_map.json (optional)

into the Modal volume used by modal_indextts2.py:
  writ-fm-indextts2-voices

Remote layout inside the volume root:
  /<filename>

When the volume is mounted at `/voices` in Modal, files appear as:
  /voices/<filename>
"""

from __future__ import annotations

import argparse
from pathlib import Path

import modal

DEFAULT_SOURCE = Path(__file__).resolve().parent / "local_voices"
VOICE_VOLUME_NAME = "writ-fm-indextts2-voices"
REMOTE_ROOT = "/"
ALLOWED_SUFFIXES = {".wav", ".mp3", ".flac", ".m4a", ".json"}


def collect_uploads(source_dir: Path) -> list[Path]:
    if not source_dir.exists():
        raise FileNotFoundError(f"source directory does not exist: {source_dir}")
    if not source_dir.is_dir():
        raise NotADirectoryError(f"source path is not a directory: {source_dir}")

    files = []
    for path in sorted(source_dir.iterdir()):
        if not path.is_file():
            continue
        if path.suffix.lower() not in ALLOWED_SUFFIXES:
            continue
        if path.name.endswith(".json") and path.name != "voice_map.json":
            continue
        files.append(path)
    return files


def build_remote_path(path: Path) -> str:
    return f"{REMOTE_ROOT}{path.name}"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Upload IndexTTS2 reference voices to a Modal volume")
    parser.add_argument(
        "--source",
        type=Path,
        default=DEFAULT_SOURCE,
        help=f"local voice directory (default: {DEFAULT_SOURCE})",
    )
    parser.add_argument(
        "--volume",
        default=VOICE_VOLUME_NAME,
        help=f"Modal volume name (default: {VOICE_VOLUME_NAME})",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print what would be uploaded without writing to Modal",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    uploads = collect_uploads(args.source)
    if not uploads:
        raise SystemExit(f"no supported files found in {args.source}")

    print(f"Found {len(uploads)} file(s) in {args.source}:")
    for path in uploads:
        print(f"  {path.name} -> {build_remote_path(path)}")

    if args.dry_run:
        print("Dry run only; no files uploaded.")
        return

    volume = modal.Volume.from_name(args.volume, create_if_missing=True)
    with volume.batch_upload(force=True) as batch:
        for path in uploads:
            batch.put_file(str(path), build_remote_path(path))

    print(f"Uploaded {len(uploads)} file(s) to Modal volume {args.volume}.")


if __name__ == "__main__":
    main()
