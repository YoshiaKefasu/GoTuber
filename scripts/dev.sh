#!/usr/bin/env bash
# dev.sh - GoTuber dev loop (Linux)
#
# Usage:
#   ./scripts/dev.sh                  # デバッグビルド + 実行
#   ./scripts/dev.sh --no-run         # ビルドのみ

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

NO_RUN=false
for arg in "$@"; do
    case "$arg" in
        --no-run)  NO_RUN=true ;;
        *)         echo "Unknown arg: $arg" >&2; exit 1 ;;
    esac
done

"$SCRIPT_DIR/build.sh" --dev --skip-test

if [ "$NO_RUN" = true ]; then
    echo "OK: build only"
    exit 0
fi

echo ""
echo "=== Running GoTuber ==="
"$ROOT_DIR/bin/gotuber"
