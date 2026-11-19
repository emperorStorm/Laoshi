#!/usr/bin/env bash
set -euo pipefail

BACKEND_ROOT=/opt/laoshi/backend
RELEASES_DIR="$BACKEND_ROOT/releases"
CURRENT_LINK="$BACKEND_ROOT/current"
SERVICE=laoshi-api
ENV_FILE=/etc/laoshi/api.env
BACKUP_DIR=/var/backups/laoshi
HEALTH=http://127.0.0.1:8787
PUBLIC_API=https://laoshi.chat/api/v1

VERSION=""
ARCHIVE=""
SHA_FILE=""

usage() {
  cat <<'EOF'
用法：sudo bash remote-deploy.sh --version <版本> --archive <包路径> --sha256-file <校验和路径>

在服务器上执行：校验包、备份数据库、迁移、原子切换 current、重启 laoshi-api 并验证。
失败时自动回滚软链；不会回退数据库迁移，不会删除旧版本或备份。
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --version) VERSION=${2:-}; shift 2 ;;
    --archive) ARCHIVE=${2:-}; shift 2 ;;
    --sha256-file) SHA_FILE=${2:-}; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数：$1" >&2; usage; exit 2 ;;
  esac
done

[[ -n $VERSION && -n $ARCHIVE && -n $SHA_FILE ]] || { usage; exit 2; }
[[ $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]] || { echo "版本号不安全：$VERSION" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "请用 root 或 sudo 运行" >&2; exit 2; }

fail() { echo "部署失败：$*" >&2; exit 1; }

TMP=$(mktemp -d /tmp/laoshi-deploy.XXXXXX)
LOG_DIR=/var/log/laoshi
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/backend-deploy-$VERSION.log"
exec > >(tee -a "$LOG") 2>&1

SWITCHED=0
ROLLED_BACK=0
PREV_TARGET=""
BACKUP=""
RELEASE_DIR=""

rollback() {
  set +e
  ROLLED_BACK=1
  echo "开始回滚到 $PREV_TARGET ..."
  local next="$BACKEND_ROOT/.rollback-$VERSION-$$"
  rm -f "$next"
  if ! ln -s "$PREV_TARGET" "$next" || ! mv -Tf "$next" "$CURRENT_LINK"; then
    echo "回滚切换失败，请立即人工介入。" >&2
    return
  fi
  systemctl restart "$SERVICE"
  sleep 2
  if systemctl is-active --quiet "$SERVICE" \
    && curl --fail --silent --max-time 5 "$HEALTH/healthz" >/dev/null; then
    echo "回滚完成，当前版本恢复为 $PREV_TARGET"
  else
    echo "回滚后服务仍不健康，请立即人工介入。" >&2
  fi
  echo "数据库迁移不会自动回退；迁移前备份：$BACKUP"
}

on_exit() {
  local rc=$?
  trap - EXIT
  if (( rc != 0 )); then
    if (( SWITCHED == 1 )) && (( ROLLED_BACK == 0 )); then
      rollback
    else
      if [[ -n $PREV_TARGET ]]; then
        echo "当前版本未切换，线上仍为 $PREV_TARGET"
      else
        echo "当前版本未切换，线上未受影响"
      fi
      if [[ -n $BACKUP ]]; then
        echo "数据库备份：$BACKUP"
      fi
      if [[ -n $RELEASE_DIR ]]; then
        echo "新版本目录保留：$RELEASE_DIR"
      fi
    fi
  fi
  rm -rf "$TMP"
  exit "$rc"
}
trap on_exit EXIT

command -v flock >/dev/null 2>&1 || fail "缺少 flock"
command -v pg_dump >/dev/null 2>&1 || fail "缺少 pg_dump，无法备份数据库"
command -v curl >/dev/null 2>&1 || fail "缺少 curl"
[[ -f $ARCHIVE ]] || fail "归档不存在：$ARCHIVE"
[[ -f $SHA_FILE ]] || fail "校验和文件不存在：$SHA_FILE"

ARCH=$(uname -m)
[[ $ARCH == x86_64 ]] || fail "服务器架构是 $ARCH，本包只支持 x86_64"
[[ -d $RELEASES_DIR ]] || fail "缺少目录：$RELEASES_DIR"
[[ -f $ENV_FILE ]] || fail "缺少环境文件：$ENV_FILE"

UNIT_TEXT=$(systemctl cat "$SERVICE" 2>/dev/null || true)
[[ -n $UNIT_TEXT ]] || fail "找不到 systemd 单元 $SERVICE；首次安装请按 deploy/README.md 第 7 节准备"

EXEC_START=$(awk '/^ExecStart=/{sub(/^ExecStart=/,""); print; exit}' <<<"$UNIT_TEXT")
WORK_DIR=$(awk '/^WorkingDirectory=/{sub(/^WorkingDirectory=/,""); print; exit}' <<<"$UNIT_TEXT")
ENV_LINE=$(awk '/^EnvironmentFile=/{sub(/^EnvironmentFile=/,""); print; exit}' <<<"$UNIT_TEXT")
[[ $EXEC_START == "$CURRENT_LINK/laoshi-api" ]] \
  || fail "现有 ExecStart=$EXEC_START，与预期 $CURRENT_LINK/laoshi-api 不一致；未做任何修改"
[[ $WORK_DIR == "$CURRENT_LINK" ]] \
  || fail "现有 WorkingDirectory=$WORK_DIR，与预期 $CURRENT_LINK 不一致；未做任何修改"
[[ $ENV_LINE == "$ENV_FILE" ]] \
  || fail "现有 EnvironmentFile=$ENV_LINE，与预期 $ENV_FILE 不一致；未做任何修改"

PERM=$(stat -c '%a' "$ENV_FILE")
OWNER=$(stat -c '%U:%G' "$ENV_FILE")
if [[ $PERM != 600 || $OWNER != root:root ]]; then
  echo "修正 $ENV_FILE 权限：$OWNER $PERM -> root:root 600"
  chown root:root "$ENV_FILE"
  chmod 600 "$ENV_FILE"
fi

EXPECTED=$(awk 'NR==1{print tolower($1)}' "$SHA_FILE")
[[ $EXPECTED =~ ^[0-9a-f]{64}$ ]] || fail "校验和文件格式不正确"
ACTUAL=$(sha256sum "$ARCHIVE")
ACTUAL=${ACTUAL%% *}
[[ $EXPECTED == "$ACTUAL" ]] || fail "SHA256 不匹配：期望 $EXPECTED，实际 $ACTUAL"

RELEASE_DIR="$RELEASES_DIR/$VERSION"
[[ ! -e $RELEASE_DIR ]] || fail "版本目录已存在，拒绝覆盖：$RELEASE_DIR"

AVAIL_KB=$(df -Pk "$BACKEND_ROOT" | awk 'NR==2{print $4}')
[[ $AVAIL_KB =~ ^[0-9]+$ ]] || fail "无法读取磁盘可用空间"
(( AVAIL_KB >= 204800 )) || fail "磁盘可用空间不足 200MB：${AVAIL_KB}KB"

exec 9>"$BACKEND_ROOT/.deploy.lock"
flock -n 9 || fail "另一个部署正在进行"

PREV_TARGET=$(readlink "$CURRENT_LINK" 2>/dev/null || true)
[[ -n $PREV_TARGET ]] || fail "$CURRENT_LINK 未指向任何版本"
[[ -x "$CURRENT_LINK/laoshi-api" ]] || fail "当前版本二进制不可执行"
echo "当前版本：$PREV_TARGET"

install -d -m 700 "$BACKUP_DIR"
TS=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP="$BACKUP_DIR/laoshi-$VERSION-$TS.dump"
echo "备份数据库到 $BACKUP ..."
if ! ( set -a; . "$ENV_FILE"; set +a; pg_dump --format=custom --file="$BACKUP" "$DATABASE_URL" ); then
  rm -f "$BACKUP"
  fail "数据库备份失败，已中止（当前版本未切换）"
fi
[[ -s $BACKUP ]] || { rm -f "$BACKUP"; fail "数据库备份为空，已中止"; }
chmod 600 "$BACKUP"

tar --warning=no-unknown-keyword -tzf "$ARCHIVE" > "$TMP/list.txt"
FIRST=$(sed -n '1p' "$TMP/list.txt")
[[ $FIRST == "laoshi-backend-$VERSION/" ]] || fail "归档顶层目录不是 laoshi-backend-$VERSION/"
grep -qx "laoshi-backend-$VERSION/laoshi-api" "$TMP/list.txt" || fail "归档缺少 laoshi-api"

mkdir "$RELEASE_DIR"
if ! tar --no-same-owner --warning=no-unknown-keyword -xzf "$ARCHIVE" -C "$RELEASE_DIR" --strip-components=1; then
  fail "解包失败"
fi
LINK=$(find "$RELEASE_DIR" -type l -print -quit)
[[ -z $LINK ]] || fail "包内包含符号链接：$LINK"
chown -R root:root "$RELEASE_DIR"
chmod -R a+rX "$RELEASE_DIR"
chmod 755 "$RELEASE_DIR/laoshi-api"
[[ -f $RELEASE_DIR/laoshi-api && -f $RELEASE_DIR/openapi.yaml && -f $RELEASE_DIR/release.json ]] \
  || fail "包内容不完整"
[[ -d $RELEASE_DIR/db/migrations ]] || fail "包内缺少 db/migrations"
MIG_COUNT=$(find "$RELEASE_DIR/db/migrations" -maxdepth 1 -type f -name '*.sql' | wc -l)
MIG_COUNT=${MIG_COUNT// /}
[[ $MIG_COUNT =~ ^[0-9]+$ ]] || fail "无法统计迁移 SQL"
(( MIG_COUNT > 0 )) || fail "包内没有迁移 SQL"
grep -q "\"version\": \"$VERSION\"" "$RELEASE_DIR/release.json" || fail "release.json 版本不一致"
grep -q '"goarch": "amd64"' "$RELEASE_DIR/release.json" || fail "release.json 不是 amd64"
if command -v file >/dev/null 2>&1; then
  BIN_INFO=$(file "$RELEASE_DIR/laoshi-api")
  case "$BIN_INFO" in
    *"ELF 64-bit LSB"*x86-64*) ;;
    *) fail "新二进制不是 x86-64 ELF：$BIN_INFO" ;;
  esac
fi

echo "执行数据库迁移（$MIG_COUNT 个 SQL，内嵌 goose）..."
if ! ( set -a; . "$ENV_FILE"; set +a; "$RELEASE_DIR/laoshi-api" -migrate ); then
  fail "数据库迁移失败，未切换 current"
fi

NEXT="$BACKEND_ROOT/.current-$VERSION-$$"
rm -f "$NEXT"
ln -s "releases/$VERSION" "$NEXT"
mv -Tf "$NEXT" "$CURRENT_LINK"
SWITCHED=1
echo "已切换 current -> releases/$VERSION"

systemctl restart "$SERVICE"
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do
  systemctl is-active --quiet "$SERVICE" && break
  sleep 1
done
systemctl is-active --quiet "$SERVICE" || fail "服务未进入 active 状态"

curl --fail --silent --show-error --max-time 5 "$HEALTH/healthz" >/dev/null \
  || fail "本机 /healthz 失败"
curl --fail --silent --show-error --max-time 5 "$HEALTH/readyz" >/dev/null \
  || fail "本机 /readyz 失败（数据库不可用？）"

CODE=$(curl --silent --show-error --max-time 10 -o "$TMP/me.json" -w '%{http_code}' "$PUBLIC_API/auth/me")
[[ $CODE == 401 ]] || fail "公网 /api/v1/auth/me 期望 401，实际 $CODE"
CTYPE=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{content_type}' "$PUBLIC_API/auth/me")
case "$CTYPE" in
  application/json*) ;;
  *) fail "公网 /api/v1/auth/me 不是 JSON：$CTYPE" ;;
esac
CODE=$(curl --silent --show-error --max-time 10 -o "$TMP/missing.txt" -w '%{http_code}' "$PUBLIC_API/definitely-missing")
[[ $CODE == 404 ]] || fail "公网未知 API 路径期望 404，实际 $CODE"
case "$(sed -n '1p' "$TMP/missing.txt" 2>/dev/null || true)" in
  *'<html'*|*'<!DOCTYPE'*) fail "未知 API 路径返回了主站 HTML，Nginx 路由可能被静态页覆盖" ;;
esac

echo "--- journalctl 最近 20 行 ---"
journalctl -u "$SERVICE" -n 20 --no-pager || true

echo "DEPLOY_OK"
echo "version=$VERSION"
echo "previous=$PREV_TARGET"
echo "backup=$BACKUP"
echo "release=$RELEASE_DIR"
echo "log=$LOG"
