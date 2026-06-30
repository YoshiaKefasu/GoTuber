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

echo "--- Installing CUDA-enabled PyTorch (cu121) ---"
"$PIP" install torch torchvision --index-url https://download.pytorch.org/whl/cu121

echo "--- Installing xformers (DA3 required dependency) ---"
# xformers は torch バージョンに厳密に一致する必要がある。
# torch 2.5.x → xformers 0.0.28.post3 (from source build)
"$PIP" install "xformers==0.0.28.post3" || echo "WARN: xformers install failed (DA3 may still work without memory-efficient attention)"

echo "--- Installing remaining requirements from $REQ_FILE ---"
"$PIP" install -r "$REQ_FILE"

# CUDA 利用可能か診断
echo
echo "--- Checking CUDA availability ---"
"$VENV_DIR/bin/python" -c "
import sys
try:
    import torch
    cuda_ok = torch.cuda.is_available()
    if cuda_ok:
        name = torch.cuda.get_device_name(0)
        ver = torch.version.cuda
        print(f'  OK: CUDA {ver} — {name}')
    else:
        print('  WARN: torch.cuda.is_available() = False. CPU inference will be used.')
        print('        If you have an NVIDIA GPU, check that CUDA toolkit matches torch CUDA version.')
        print('        Manual fix: .venv-depth/bin/pip install torch --index-url https://download.pytorch.org/whl/cu121')
except ImportError:
    print('  WARN: torch not installed — depth generation will not work.')
except Exception as e:
    print(f'  WARN: CUDA check failed: {e}')
"

echo
echo "Phase 3.6 Depth Anything v3 environment setup complete. Activate: source .venv-depth/bin/activate"
