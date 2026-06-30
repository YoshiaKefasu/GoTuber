#!/usr/bin/env bash
set -euo pipefail

# setup-depth.sh - GoTuber Phase 3.6 Depth Anything v3 Python environment setup (Linux/WSL)

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
VENV_DIR="$ROOT_DIR/.venv-depth"
REQ_FILE="$ROOT_DIR/tools/requirements-depth.txt"

cd "$ROOT_DIR"

echo "=== Phase 3.6 Depth Anything v3 environment setup (Linux/WSL) ==="

if ! command -v python3 >/dev/null 2>&1; then
  echo "ERROR: python3 not found. Install Python 3.10+ first." >&2
  exit 1
fi

PY_VERSION="$(python3 -c 'import sys; print(f"{sys.version_info.major}.{sys.version_info.minor}")')"
PY_MAJOR="${PY_VERSION%%.*}"
PY_MINOR="${PY_VERSION##*.}"
if [[ "$PY_MAJOR" -lt 3 || ( "$PY_MAJOR" -eq 3 && "$PY_MINOR" -lt 10 ) ]]; then
  echo "ERROR: Python 3.10+ required. Found $PY_VERSION" >&2
  exit 1
fi

if [[ "${1:-}" == "--force" && -d "$VENV_DIR" ]]; then
  echo "--- Removing existing venv ---"
  rm -rf "$VENV_DIR"
fi

if [[ -d "$VENV_DIR" ]]; then
  echo ".venv-depth/ already exists — skipping. Use --force to recreate."
  echo "Activate: source .venv-depth/bin/activate"
  exit 0
fi

echo "--- Creating venv: $VENV_DIR ---"
python3 -m venv "$VENV_DIR"

PIP="$VENV_DIR/bin/pip"

echo "--- Upgrading pip / wheel / setuptools ---"
"$PIP" install --upgrade pip wheel setuptools

echo "--- Installing requirements from $REQ_FILE (prefer-binary) ---"
"$PIP" install --prefer-binary -r "$REQ_FILE"

echo
echo "Phase 3.6 Depth Anything v3 environment setup complete. Activate: source .venv-depth/bin/activate"
