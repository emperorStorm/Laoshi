#!/usr/bin/env bash
set -euo pipefail

fail() { echo "部署中止：$*" >&2; exit 1; }

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

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
ROOT=$(find_root) || fail "未找到 Laoshi 仓库根目录"
HOST=${LAOSHI_SSH_HOST:-ecs-114-55-112-83}
INBOX=${LAOSHI_REMOTE_INBOX:-/home/package}
SERVICE=laoshi-api
BUILD=1
MODE=deploy
CONFIRM=0
VERSION=""
ARTIFACT=""
CHECKSUM=""

usage() {
  cat <<'EOF'
用法：
  deploy-backend.sh --preflight-only          构建并只读预检，不部署（默认推荐先跑这个）
  deploy-backend.sh --build-only              只构建，不连接服务器
  deploy-backend.sh --skip-build --version V --yes
                                              用已有产物上传并部署
可选参数：
  --version <版本>   指定已有版本（配合 --skip-build）
  --artifact <路径>  指定已有归档
  --host <别名>      默认 ecs-114-55-112-83，也可用 LAOSHI_SSH_HOST
  --inbox <目录>     默认 /home/package，也可用 LAOSHI_REMOTE_INBOX
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --preflight-only) MODE=preflight-only; shift ;;
    --build-only) MODE=build-only; shift ;;
    --skip-build) BUILD=0; shift ;;
    --yes) CONFIRM=1; shift ;;
    --version) VERSION=${2:-}; shift 2 ;;
    --artifact) ARTIFACT=${2:-}; shift 2 ;;
    --host) HOST=${2:-}; shift 2 ;;
    --inbox) INBOX=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) fail "未知参数：$1" ;;
  esac
done

BUILD_LOG=""
cleanup_build_log() {
  if [[ -n ${BUILD_LOG:-} ]]; then
    rm -f "$BUILD_LOG"
  fi
}
trap cleanup_build_log EXIT

if (( BUILD == 1 )); then
  BUILD_LOG=$(mktemp "${TMPDIR:-/tmp}/laoshi-build-log.XXXXXX")
  bash "$SCRIPT_DIR/build-backend.sh" | tee "$BUILD_LOG"
  VERSION=$(awk -F= '/^VERSION=/{print $2; exit}' "$BUILD_LOG")
  ARTIFACT=$(awk '/^ARTIFACT=/{sub(/^ARTIFACT=/,""); print; exit}' "$BUILD_LOG")
  CHECKSUM=$(awk '/^CHECKSUM=/{sub(/^CHECKSUM=/,""); print; exit}' "$BUILD_LOG")
  [[ -n $VERSION && -n $ARTIFACT && -n $CHECKSUM ]] || fail "无法从构建输出解析版本或产物路径"
else
  [[ -n $VERSION ]] || fail "--skip-build 必须同时提供 --version"
  [[ -n $ARTIFACT ]] || ARTIFACT="$ROOT/artifacts/backend/laoshi-backend-$VERSION-linux-amd64.tar.gz"
  [[ -n $CHECKSUM ]] || CHECKSUM="$ARTIFACT.sha256"
fi

[[ -f $ARTIFACT ]] || fail "归档不存在：$ARTIFACT"
[[ -f $CHECKSUM ]] || fail "校验和文件不存在：$CHECKSUM"

if [[ $MODE == build-only ]]; then
  echo "仅构建完成，未连接服务器。"
  echo "产物：$ARTIFACT"
  echo "校验和：$CHECKSUM"
  exit 0
fi

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3 -o LogLevel=ERROR)
remote() { ssh "${SSH_OPTS[@]}" "$HOST" "$1"; }

if ! remote true; then
  ARCH_NAME=$(basename "$ARTIFACT")
  SHA_NAME=$(basename "$CHECKSUM")
  cat <<EOF

上传前检查失败：无法通过 SSH 连接 $HOST。
发布包已生成且未上传、未部署，可以保留待网络恢复后重试：
  归档：$ARTIFACT
  校验：$CHECKSUM

请检查本机网络/VPN 或云安全组 22 端口后重试。不要在不确定网络状态时反复重试。
网络恢复后可直接用 --skip-build --version $VERSION --yes 重新执行；
也可以在终端手动上传并运行远端脚本：
  scp "$ARTIFACT" "$CHECKSUM" "$HOST:$INBOX/"
  scp "$SCRIPT_DIR/remote-deploy.sh" "$HOST:$INBOX/deploy-backend-$VERSION.sh"
  ssh -t "$HOST" "sudo bash $INBOX/deploy-backend-$VERSION.sh --version $VERSION --archive $INBOX/$ARCH_NAME --sha256-file $INBOX/$SHA_NAME"
EOF
  exit 4
fi

ARCH=$(remote 'uname -m')
ACTIVE=$(remote "systemctl is-active $SERVICE 2>/dev/null || true")
EXEC=$(remote "systemctl cat $SERVICE 2>/dev/null | sed -n 's/^ExecStart=//p'")
CURRENT=$(remote 'readlink -f /opt/laoshi/backend/current 2>/dev/null || true')
INBOX_OK=$(remote "test -d '$INBOX' && echo yes || echo no")

[[ $ARCH == x86_64 ]] || fail "远端架构是 $ARCH，不是 x86_64"
[[ $INBOX_OK == yes ]] || fail "远端目录不存在：$INBOX"
[[ $EXEC == "/opt/laoshi/backend/current/laoshi-api" ]] \
  || fail "远端 ExecStart 与预期不一致：[$EXEC]；未上传、未修改"
[[ $ACTIVE == active ]] || echo "警告：$SERVICE 当前状态为 [$ACTIVE]，部署脚本会重启它。" >&2

SHA_VALUE=$(awk 'NR==1{print $1}' "$CHECKSUM")

cat <<EOF

================ 部署计划 ================
版本：$VERSION
产物：$ARTIFACT
SHA256：$SHA_VALUE
远端：$HOST（$ARCH）
远端当前：${CURRENT:-未知}
上传目录：$INBOX
动作：上传 -> pg_dump 备份 -> 解包 -> 迁移 -> 原子切换 current -> 重启 $SERVICE -> healthz/readyz + 公网 401/404 校验
失败：切换后验证失败会自动回滚软链并重启旧版本；数据库迁移不回退
==========================================
EOF

if [[ $MODE == preflight-only || $CONFIRM != 1 ]]; then
  echo "尚未部署。确认这是预期的生产变更多后执行："
  echo "  bash $SCRIPT_DIR/deploy-backend.sh --skip-build --version $VERSION --yes"
  exit 0
fi

ARCH_NAME=$(basename "$ARTIFACT")
SHA_NAME=$(basename "$CHECKSUM")
REMOTE_SCRIPT="$INBOX/deploy-backend-$VERSION.sh"

echo "上传到 $HOST:$INBOX ..."
if ! scp "${SSH_OPTS[@]}" "$ARTIFACT" "$CHECKSUM" "$HOST:$INBOX/"; then
  fail "上传归档失败；服务器未做修改，产物保留在 $ARTIFACT"
fi
if ! scp "${SSH_OPTS[@]}" "$SCRIPT_DIR/remote-deploy.sh" "$HOST:$REMOTE_SCRIPT"; then
  fail "上传远端脚本失败；服务器未做修改，产物保留在 $ARTIFACT"
fi

echo "执行远端部署 ..."
set +e
ssh "${SSH_OPTS[@]}" "$HOST" \
  "sudo -n bash '$REMOTE_SCRIPT' --version '$VERSION' --archive '$INBOX/$ARCH_NAME' --sha256-file '$INBOX/$SHA_NAME'"
RC=$?
set -e

if (( RC != 0 )); then
  echo "远端部署失败（exit $RC）。" >&2
  echo "不要重复部署同一版本（版本目录已存在）；先看日志：$HOST:/var/log/laoshi/backend-deploy-$VERSION.log" >&2
  exit "$RC"
fi

CODE=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{http_code}' https://laoshi.chat/api/v1/auth/me || true)
if [[ $CODE != 401 ]]; then
  echo "警告：本机公网复核 /api/v1/auth/me 返回 [$CODE]，远端已通过校验，请检查本机网络或稍后复测。" >&2
fi

echo
echo "部署完成"
echo "版本：$VERSION"
echo "产物：$ARTIFACT"
echo "SHA256：$SHA_VALUE"
echo "远端日志：$HOST:/var/log/laoshi/backend-deploy-$VERSION.log"
echo "Nginx 未改动；旧版本、数据库备份和上传归档均保留，未自动清理。"
