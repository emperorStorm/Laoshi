#!/usr/bin/env bash
set -euo pipefail

fail() { echo "前端部署中止：$*" >&2; exit 1; }

find_root() {
  local dir
  dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
  while [[ $dir != / ]]; do
    if [[ -f "$dir/server/go.mod" && -f "$dir/package.json" ]]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir=$(dirname "$dir")
  done
  return 1
}

ROOT=$(find_root) || fail "未找到 Laoshi 仓库根目录"
HOST=${LAOSHI_DEPLOY_HOST:-114.55.112.83}
DEPLOY_USER=${LAOSHI_DEPLOY_USER:-deploy}
KEY=${LAOSHI_DEPLOY_KEY:-$HOME/.ssh/laoshi_deploy_ed25519}
INBOX=${LAOSHI_REMOTE_INBOX:-/home/package}
REMOTE_SCRIPT=/usr/local/sbin/laoshi-deploy-web

BUILD=1
BUILD_ONLY=0
DRY=0
VERSION=""
ARCHIVE=""
CHECKSUM=""

usage() {
  cat <<'EOF'
用法：
  deploy-web.sh --version v1.2.0                构建、上传并部署前端
  deploy-web.sh --version v1.2.0 --dry-run      构建、上传，远端只校验不切换
  deploy-web.sh --version v1.2.0 --build-only   只构建，不连服务器
  deploy-web.sh --version v1.2.0 --skip-build --archive <包> --sha256 <校验和>
                                                用已有产物部署
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --build) BUILD=1; shift ;;
    --skip-build) BUILD=0; shift ;;
    --build-only) BUILD_ONLY=1; shift ;;
    --dry-run) DRY=1; shift ;;
    --version) VERSION=${2:-}; shift 2 ;;
    --archive) ARCHIVE=${2:-}; shift 2 ;;
    --sha256) CHECKSUM=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知参数：$1" ;;
  esac
done

if [[ -z $VERSION ]]; then
  VERSION="v$(awk -F'"' '/"version":/ { print $4; exit }' "$ROOT/package.json")"
fi
[[ $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]] || fail "版本号不安全：$VERSION"

if (( BUILD == 1 )); then
  NODE_MAJOR=$(node -p 'process.versions.node.split(".")[0]' 2>/dev/null || echo 0)
  (( NODE_MAJOR >= 22 )) || fail "需要 Node 22+，当前 $(node -v 2>/dev/null || echo 未安装)。可先执行 nvm use 22"
  if [[ ! -d "$ROOT/node_modules" ]]; then
    ( cd "$ROOT" && npm ci )
  fi
  ( cd "$ROOT" && SITE_URL="${SITE_URL:-https://laoshi.chat}" RELEASE_ID="$VERSION" npm run package:deploy )
  ARCHIVE="$ROOT/artifacts/laoshi-$VERSION.tar.gz"
  CHECKSUM="$ARCHIVE.sha256"
  [[ -f $ARCHIVE ]] || fail "构建结束但没有找到归档：$ARCHIVE"
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "$(dirname "$ARCHIVE")" && sha256sum "$(basename "$ARCHIVE")" > "$(basename "$CHECKSUM")" )
  else
    ( cd "$(dirname "$ARCHIVE")" && shasum -a 256 "$(basename "$ARCHIVE")" > "$(basename "$CHECKSUM")" )
  fi
else
  [[ -n $ARCHIVE ]] || ARCHIVE="$ROOT/artifacts/laoshi-$VERSION.tar.gz"
  [[ -n $CHECKSUM ]] || CHECKSUM="$ARCHIVE.sha256"
fi

[[ -f $ARCHIVE ]] || fail "归档不存在：$ARCHIVE"
[[ -f $CHECKSUM ]] || fail "校验和文件不存在：$CHECKSUM"

if (( BUILD_ONLY == 1 )); then
  echo "仅构建完成，未连接服务器。"
  echo "产物：$ARCHIVE"
  echo "校验和：$CHECKSUM"
  exit 0
fi

[[ -f $KEY ]] || fail "找不到部署私钥：$KEY"
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=yes -o ServerAliveInterval=5 -o ServerAliveCountMax=3 -o LogLevel=ERROR -i "$KEY")
SSH_TARGET="$DEPLOY_USER@$HOST"
ARCH_NAME=$(basename "$ARCHIVE")
SHA_NAME=$(basename "$CHECKSUM")
REMOTE_ARCHIVE="$INBOX/$ARCH_NAME"
REMOTE_SHA="$INBOX/$SHA_NAME"
REMOTE_ARGS=(--version "$VERSION" --archive "$REMOTE_ARCHIVE" --sha256-file "$REMOTE_SHA")
if (( DRY == 1 )); then
  REMOTE_ARGS+=(--dry-run)
fi

echo "上传前端包到 $SSH_TARGET:$INBOX ..."
if ! scp "${SSH_OPTS[@]}" "$ARCHIVE" "$CHECKSUM" "$SSH_TARGET:$INBOX/"; then
  fail "上传失败，服务器未修改；产物保留在 $ARCHIVE"
fi

echo "执行远端前端部署 ..."
set +e
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" "sudo -n $REMOTE_SCRIPT ${REMOTE_ARGS[*]}"
RC=$?
set -e

if (( RC != 0 )); then
  echo "前端部署失败（exit $RC）。" >&2
  echo "查看日志：$HOST:/var/log/laoshi/web-deploy-$VERSION.log" >&2
  exit "$RC"
fi

echo "前端部署完成：$VERSION"
