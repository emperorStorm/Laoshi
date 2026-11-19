#!/usr/bin/env bash
set -euo pipefail

fail() { echo "API 部署中止：$*" >&2; exit 1; }

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
REMOTE_SCRIPT=/usr/local/sbin/laoshi-deploy-api

BUILD=0
DRY=0
VERSION=""
ARCHIVE=""
CHECKSUM=""

usage() {
  cat <<'EOF'
用法：
  deploy-api.sh --version v1.2.0 --build        构建并部署后端
  deploy-api.sh --version v1.2.0 --build --dry-run
                                                只构建，不上传不部署
  deploy-api.sh --version v1.2.0 --skip-build --archive <包> --sha256 <校验和>
                                                用已有产物部署
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --build) BUILD=1; shift ;;
    --skip-build) BUILD=0; shift ;;
    --dry-run|--build-only) DRY=1; shift ;;
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

BUILD_LOG=""
cleanup() {
  if [[ -n ${BUILD_LOG:-} ]]; then
    rm -f "$BUILD_LOG"
  fi
}
trap cleanup EXIT

if (( BUILD == 1 )); then
  BUILD_LOG=$(mktemp "${TMPDIR:-/tmp}/laoshi-api-build.XXXXXX")
  RELEASE_ID="$VERSION" bash "$ROOT/.qoder/skills/deploy-laoshi-backend/scripts/build-backend.sh" | tee "$BUILD_LOG"
  ARCHIVE=$(awk '/^ARTIFACT=/{sub(/^ARTIFACT=/,""); print; exit}' "$BUILD_LOG")
  CHECKSUM=$(awk '/^CHECKSUM=/{sub(/^CHECKSUM=/,""); print; exit}' "$BUILD_LOG")
else
  [[ -n $ARCHIVE ]] || ARCHIVE="$ROOT/artifacts/backend/laoshi-backend-$VERSION-linux-amd64.tar.gz"
  [[ -n $CHECKSUM ]] || CHECKSUM="$ARCHIVE.sha256"
fi

[[ -f $ARCHIVE ]] || fail "归档不存在：$ARCHIVE"
[[ -f $CHECKSUM ]] || fail "校验和文件不存在：$CHECKSUM"

if (( DRY == 1 )); then
  echo "dry-run：后端只构建，不上传不部署。"
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

echo "上传后端包到 $SSH_TARGET:$INBOX ..."
if ! scp "${SSH_OPTS[@]}" "$ARCHIVE" "$CHECKSUM" "$SSH_TARGET:$INBOX/"; then
  fail "上传失败，服务器未修改；产物保留在 $ARCHIVE"
fi

echo "执行远端后端部署 ..."
set +e
ssh "${SSH_OPTS[@]}" "$SSH_TARGET" \
  "sudo -n $REMOTE_SCRIPT --version '$VERSION' --archive '$REMOTE_ARCHIVE' --sha256-file '$REMOTE_SHA'"
RC=$?
set -e

if (( RC != 0 )); then
  echo "后端部署失败（exit $RC）。" >&2
  echo "查看日志：$HOST:/var/log/laoshi/backend-deploy-$VERSION.log" >&2
  exit "$RC"
fi

echo "后端部署完成：$VERSION"
