#!/usr/bin/env bash
# deploy-kasou.sh - GoTuber KASOU deploy (Linux)
#
# Usage:
#   ./scripts/deploy-kasou.sh                  # デフォルト (release build + scp + restart)
#   ./scripts/deploy-kasou.sh --no-restart     # デプロイのみ、systemd 再起動なし
#   KASOU_HOST=kasou.local ./scripts/deploy-kasou.sh
#
# Requirements:
#   - ssh (KASOU への接続)
#   - scp
#   - ~/.ssh/config に "Host kasou" エントリ (KASOU_HOST 上書き可能)
#   - systemd unit: gotuber.service が KASOU にある (手動作成)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

KASOU_HOST="${KASOU_HOST:-kasou}"
KASOU_USER="${KASOU_USER:-yosia}"
KASOU_DEPLOY_DIR="${KASOU_DEPLOY_DIR:-/opt/gotuber}"
KASOU_SERVICE="${KASOU_SERVICE:-gotuber.service}"

if [ -t 1 ]; then
    CYAN='\033[0;36m'
    YELLOW='\033[0;33m'
    GREEN='\033[0;32m'
    RED='\033[0;31m'
    NC='\033[0m'
else
    CYAN='' YELLOW='' GREEN='' RED='' NC=''
fi

NO_RESTART=false
for arg in "$@"; do
    case "$arg" in
        --no-restart)  NO_RESTART=true ;;
        *)             echo "Unknown arg: $arg" >&2; exit 1 ;;
    esac
done

# 値のバリデーション (シェルインジェクション対策)
if [[ ! "$KASOU_SERVICE" =~ ^[a-zA-Z0-9._-]+$ ]]; then
    echo "ERROR: invalid KASOU_SERVICE: $KASOU_SERVICE (英数 + . _ - のみ)" >&2
    exit 1
fi
case "$KASOU_DEPLOY_DIR" in
    /opt/*|/srv/*) ;;
    *) echo "ERROR: KASOU_DEPLOY_DIR must start with /opt or /srv: $KASOU_DEPLOY_DIR" >&2; exit 1 ;;
esac
if [[ ! "$KASOU_HOST" =~ ^[a-zA-Z0-9._-]+$ ]]; then
    echo "ERROR: invalid KASOU_HOST: $KASOU_HOST" >&2
    exit 1
fi

echo -e "${CYAN}=== GoTuber KASOU deploy ===${NC}"
echo "Host: $KASOU_USER@$KASOU_HOST"
echo "Deploy dir: $KASOU_DEPLOY_DIR"
echo ""

# 1) Linux ビルド
echo -e "${YELLOW}--- Building for Linux ---${NC}"
"$SCRIPT_DIR/build.sh"

# 2) 既存プロセス停止 (no-restart が指定されてない時)
if [ "$NO_RESTART" = false ]; then
    echo -e "${YELLOW}--- Stopping systemd service ---${NC}"
    ssh "$KASOU_HOST" "sudo systemctl stop $KASOU_SERVICE 2>/dev/null || true"
fi

# 3) デプロイ
echo -e "${YELLOW}--- Deploying binary ---${NC}"
ssh "$KASOU_HOST" "mkdir -p $KASOU_DEPLOY_DIR"
scp "bin/gotuber" "$KASOU_HOST:$KASOU_DEPLOY_DIR/gotuber"
ssh "$KASOU_HOST" "chmod +x $KASOU_DEPLOY_DIR/gotuber"

# assets と config も同期 (初回 or 変更検知時)
ssh "$KASOU_HOST" "mkdir -p $KASOU_DEPLOY_DIR/assets $KASOU_DEPLOY_DIR/config"
# WARNING: --delete により KASOU 側にしかないファイルも削除されます
rsync -av --delete "assets/" "$KASOU_HOST:$KASOU_DEPLOY_DIR/assets/"
rsync -av --delete "config/" "$KASOU_HOST:$KASOU_DEPLOY_DIR/config/"

# 4) systemd 再起動
if [ "$NO_RESTART" = false ]; then
    echo -e "${YELLOW}--- Restarting systemd service ---${NC}"
    ssh "$KASOU_HOST" "sudo systemctl restart $KASOU_SERVICE"
    sleep 1
    ssh "$KASOU_HOST" "sudo systemctl status $KASOU_SERVICE --no-pager | head -20"
fi

echo ""
echo -e "${GREEN}OK: deployed to $KASOU_HOST:$KASOU_DEPLOY_DIR${NC}"
