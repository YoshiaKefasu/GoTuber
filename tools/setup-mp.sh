#!/usr/bin/env bash
# setup-mp.sh - GoTuber Phase 2 MediaPipe Python 環境セットアップ (Linux / WSL / macOS)
#
# Usage:
#   ./tools/setup-mp.sh                # .venv-mp/ を作成 + tools/requirements-mp.txt を pip install
#   ./tools/setup-mp.sh --force        # 既存 .venv-mp/ を削除して再作成
#
# Requirements:
#   - Python 3.9+ (mediapipe 0.10.x 要件。3.13+ は pip install 時に弾かれる可能性あり)
#   - python3-venv パッケージ (Debian/Ubuntu: sudo apt install python3-venv)
#
# 詳細: docs/PHASE2.md Section 4.4

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# カラー出力 (可能な場合)
if [ -t 1 ]; then
    CYAN='\033[0;36m'
    YELLOW='\033[0;33m'
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    NC='\033[0m'
else
    CYAN='' YELLOW='' GREEN='' RED='' NC=''
fi

# 引数処理
FORCE=false
for arg in "$@"; do
    case "$arg" in
        --force)  FORCE=true ;;
        *)        echo "Unknown arg: $arg" >&2; exit 1 ;;
    esac
done

# Python 3.9+ 検出 (python3 → python の順)
PYTHON_BIN=""
for candidate in python3 python; do
    if command -v "$candidate" >/dev/null 2>&1; then
        if "$candidate" -c "import sys; sys.exit(0 if sys.version_info >= (3, 9) else 1)" 2>/dev/null; then
            PYTHON_BIN="$(command -v "$candidate")"
            break
        fi
    fi
done

if [ -z "$PYTHON_BIN" ]; then
    echo -e "${RED}ERROR: Python 3.9+ が見つかりません。python3 / python を PATH に追加してください。${NC}" >&2
    echo -e "${RED}  Debian/Ubuntu: sudo apt install python3 python3-venv python3-pip${NC}" >&2
    exit 1
fi

VENV_DIR="$ROOT_DIR/.venv-mp"
REQUIREMENTS="$ROOT_DIR/tools/requirements-mp.txt"
MODEL_PATH="$ROOT_DIR/assets/models/face_landmarker.task"

if [ ! -f "$REQUIREMENTS" ]; then
    echo -e "${RED}ERROR: $REQUIREMENTS が見つかりません。${NC}" >&2
    exit 1
fi

if [ ! -f "$MODEL_PATH" ]; then
    echo -e "${RED}ERROR: 同梱モデル $MODEL_PATH が見つかりません。${NC}" >&2
    echo -e "${RED}  リポジトリを取り直すか、docs/PHASE2.md Section 2.9 を確認してください。${NC}" >&2
    exit 1
fi

if [ "$FORCE" = true ] && [ -d "$VENV_DIR" ]; then
    echo -e "${YELLOW}--- Removing existing venv ---${NC}"
    rm -rf "$VENV_DIR"
fi

if [ -d "$VENV_DIR" ]; then
    echo -e "${GREEN}.venv-mp/ already exists — skipping. Use --force to recreate.${NC}"
    echo "Activate: source $VENV_DIR/bin/activate"
    exit 0
fi

echo -e "${CYAN}=== Phase 2 MediaPipe environment setup (Unix) ===${NC}"

echo -e "${YELLOW}--- Creating venv: $VENV_DIR (using $PYTHON_BIN) ---${NC}"
"$PYTHON_BIN" -m venv "$VENV_DIR"

echo -e "${YELLOW}--- Upgrading pip / wheel / setuptools ---${NC}"
"$VENV_DIR/bin/pip" install --upgrade pip wheel setuptools

echo -e "${YELLOW}--- Installing requirements from tools/requirements-mp.txt (prefer-binary) ---${NC}"
"$VENV_DIR/bin/pip" install --prefer-binary -r "$REQUIREMENTS"

echo -e "${YELLOW}--- Bundled MediaPipe model ---${NC}"
ls -lh "$MODEL_PATH"

echo ""
echo -e "${GREEN}Phase 2 MediaPipe 環境セットアップ完了。Activate: source .venv-mp/bin/activate${NC}"
