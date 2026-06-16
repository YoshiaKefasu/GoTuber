#!/usr/bin/env bash
# build.sh - GoTuber Linux/WSL build
#
# Usage:
#   ./scripts/build.sh                # リリースビルド
#   ./scripts/build.sh --dev          # デバッグビルド (-ldflags なし)
#   ./scripts/build.sh --clean        # ビルド前に bin/ 削除
#   ./scripts/build.sh --skip-test    # テストスキップ
#
# Requirements:
#   - Go 1.25+ (実要件は go.mod の go ディレクティブを参照。Phase 1.9 時点で 1.26 系)
#   - gcc + libasound2-dev (malgo CGo)
#     sudo apt install gcc libasound2-dev build-essential

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

# WSL で Windows 版 go が PATH 先頭に来る現象を回避 (interop 無効)
# 優先: /usr/local/go/bin (Linux native) > /usr/bin (apt) > Windows binary
if [ -x "/usr/local/go/bin/go" ]; then
    export PATH="/usr/local/go/bin:$PATH"
    GO_BIN="/usr/local/go/bin/go"
elif command -v go >/dev/null 2>&1; then
    GO_BIN="$(command -v go)"
else
    echo "ERROR: go not found" >&2
    exit 1
fi

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

echo -e "${CYAN}=== GoTuber build (Linux) ===${NC}"

DEV=false
CLEAN=false
SKIP_TEST=false
for arg in "$@"; do
    case "$arg" in
        --dev)        DEV=true ;;
        --clean)      CLEAN=true ;;
        --skip-test)  SKIP_TEST=true ;;
        *)            echo "Unknown arg: $arg" >&2; exit 1 ;;
    esac
done

if [ "$CLEAN" = true ] && [ -d "bin" ]; then
    echo "Cleaning bin/"
    rm -rf "bin"
fi

mkdir -p "bin"

if [ "$SKIP_TEST" = false ]; then
    echo -e "${YELLOW}--- go test ---${NC}"
    "$GO_BIN" test ./...
fi

echo -e "${YELLOW}--- go vet ---${NC}"
"$GO_BIN" vet ./...

LDFLAGS_ARGS=()
if [ "$DEV" = false ]; then
    # リリース: シンボル & DWARF 削除
    LDFLAGS_ARGS=(-ldflags "-s -w")
fi

echo -e "${YELLOW}--- Linux build ---${NC}"
"$GO_BIN" build "${LDFLAGS_ARGS[@]}" -o bin/gotuber ./cmd/gotuber

SIZE=$(du -h "bin/gotuber" | cut -f1)
echo ""
echo -e "${GREEN}OK: bin/gotuber ($SIZE)${NC}"
