#!/usr/bin/env bash
set -euo pipefail

WEB_ROOT=/opt/laoshi
RELEASES_DIR="$WEB_ROOT/releases"
CURRENT_LINK="$WEB_ROOT/current"
NGINX_CONF=/etc/nginx/conf.d/laoshi.conf
EXPECTED_NGINX_ROOT=/opt/laoshi/current/web
LOG_DIR=/var/log/laoshi
PUBLIC_URL=https://laoshi.chat

VERSION=""
ARCHIVE=""
SHA_FILE=""
DRY=0

usage() {
  cat <<'EOF'
用法：sudo bash remote-deploy-web.sh --version <版本> --archive <包路径> --sha256-file <校验和> [--dry-run]

校验静态包、解包到 /opt/laoshi/releases/<版本>、原子切换 /opt/laoshi/current，
验证首页/作品页/404/资源/API。--dry-run 只解包校验，不切换。
EOF
}

while (( $# > 0 )); do
  case "$1" in
    --version) VERSION=${2:-}; shift 2 ;;
    --archive) ARCHIVE=${2:-}; shift 2 ;;
    --sha256-file) SHA_FILE=${2:-}; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数：$1" >&2; usage; exit 2 ;;
  esac
done

[[ -n $VERSION && -n $ARCHIVE && -n $SHA_FILE ]] || { usage; exit 2; }
[[ $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]] || { echo "版本号不安全：$VERSION" >&2; exit 2; }
[[ $EUID -eq 0 ]] || { echo "请用 root 或 sudo 运行" >&2; exit 2; }

fail() { echo "前端部署失败：$*" >&2; exit 1; }

TMP=$(mktemp -d /tmp/laoshi-web-deploy.XXXXXX)
mkdir -p "$LOG_DIR"
if (( DRY == 1 )); then
  LOG="$LOG_DIR/web-deploy-$VERSION-dryrun.log"
else
  LOG="$LOG_DIR/web-deploy-$VERSION.log"
fi
exec > >(tee -a "$LOG") 2>&1

SWITCHED=0
ROLLED_BACK=0
PREV_TARGET=""
RELEASE_DIR=""

verify_site() {
  local site_root=$1
  local work_path work asset url code
  curl --fail --silent --show-error --retry 2 --retry-delay 1 --max-time 10 "$PUBLIC_URL/" >/dev/null \
    || return 1
  work_path=$(find "$site_root/web/work" -mindepth 1 -maxdepth 1 -type d -print -quit 2>/dev/null || true)
  if [[ -n $work_path ]]; then
    work=${work_path##*/}
    curl --fail --silent --show-error --retry 2 --retry-delay 1 --max-time 10 "$PUBLIC_URL/work/$work/" >/dev/null \
      || return 1
  fi
  code=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{http_code}' "$PUBLIC_URL/definitely-missing")
  [[ $code == 404 ]] || return 1
  asset=$(find "$site_root/web/assets" -type f -name '*.js' -print -quit 2>/dev/null || true)
  if [[ -n $asset ]]; then
    url=${asset#"$site_root/web"}
    curl --fail --silent --show-error --retry 2 --retry-delay 1 --max-time 10 "$PUBLIC_URL$url" >/dev/null \
      || return 1
  fi
  code=$(curl --silent --show-error --max-time 10 -o /dev/null -w '%{http_code}' "$PUBLIC_URL/api/v1/auth/me")
  [[ $code == 401 ]] || return 1
}

rollback() {
  set +e
  ROLLED_BACK=1
  echo "开始回滚前端到 $PREV_TARGET ..."
  local next="$WEB_ROOT/.rollback-$VERSION-$$"
  rm -f "$next"
  if ! ln -s "$PREV_TARGET" "$next" || ! mv -Tf "$next" "$CURRENT_LINK"; then
    echo "前端回滚切换失败，请立即人工介入。" >&2
    return
  fi
  if verify_site "$CURRENT_LINK"; then
    echo "前端回滚完成，当前版本恢复为 $PREV_TARGET"
  else
    echo "前端回滚后验证不通过，请立即人工介入。" >&2
  fi
}

on_exit() {
  local rc=$?
  trap - EXIT
  if (( rc != 0 )) && (( SWITCHED == 1 )) && (( ROLLED_BACK == 0 )); then
    rollback
  elif (( rc != 0 )); then
    echo "前端未切换，线上仍为 ${PREV_TARGET:-未知}"
  fi
  rm -rf "$TMP"
  exit "$rc"
}
trap on_exit EXIT

command -v flock >/dev/null 2>&1 || fail "缺少 flock"
command -v curl >/dev/null 2>&1 || fail "缺少 curl"
command -v sha256sum >/dev/null 2>&1 || fail "缺少 sha256sum"
[[ -f $ARCHIVE ]] || fail "归档不存在：$ARCHIVE"
[[ -f $SHA_FILE ]] || fail "校验和文件不存在：$SHA_FILE"
[[ -d $WEB_ROOT ]] || fail "缺少目录：$WEB_ROOT"
[[ -d $RELEASES_DIR ]] || fail "缺少目录：$RELEASES_DIR"
[[ -f $NGINX_CONF ]] || fail "缺少 Nginx 配置：$NGINX_CONF"
grep -Fq "root $EXPECTED_NGINX_ROOT;" "$NGINX_CONF" \
  || fail "Nginx root 不是 $EXPECTED_NGINX_ROOT，请先完成前端目录迁移"

EXPECTED=$(awk 'NR==1{print tolower($1)}' "$SHA_FILE")
[[ $EXPECTED =~ ^[0-9a-f]{64}$ ]] || fail "校验和文件格式不正确"
ACTUAL=$(sha256sum "$ARCHIVE")
ACTUAL=${ACTUAL%% *}
[[ $EXPECTED == "$ACTUAL" ]] || fail "SHA256 不匹配：期望 $EXPECTED，实际 $ACTUAL"

RELEASE_DIR="$RELEASES_DIR/$VERSION"
[[ ! -e $RELEASE_DIR ]] || fail "版本目录已存在，拒绝覆盖：$RELEASE_DIR"

AVAIL_KB=$(df -Pk "$WEB_ROOT" | awk 'NR==2{print $4}')
[[ $AVAIL_KB =~ ^[0-9]+$ ]] || fail "无法读取磁盘可用空间"
(( AVAIL_KB >= 204800 )) || fail "磁盘可用空间不足 200MB：${AVAIL_KB}KB"

exec 9>"$WEB_ROOT/.web-deploy.lock"
flock -n 9 || fail "另一个前端部署正在进行"

PREV_TARGET=$(readlink "$CURRENT_LINK" 2>/dev/null || true)
[[ -n $PREV_TARGET ]] || fail "$CURRENT_LINK 未指向任何版本"
[[ -f "$CURRENT_LINK/web/index.html" ]] || fail "当前版本缺少 web/index.html"
echo "当前前端版本：$PREV_TARGET"

extract_and_validate() {
  local target=$1
  tar --warning=no-unknown-keyword -tzf "$ARCHIVE" > "$TMP/list.txt"
  grep -qx 'web/index.html' "$TMP/list.txt" || fail "归档缺少 web/index.html"
  grep -qx 'web/404.html' "$TMP/list.txt" || fail "归档缺少 web/404.html"
  grep -qx 'release.json' "$TMP/list.txt" || fail "归档缺少 release.json"
  if ! tar --no-same-owner --warning=no-unknown-keyword -xzf "$ARCHIVE" -C "$target"; then
    fail "解包失败"
  fi
  local link
  link=$(find "$target" -type l -print -quit)
  [[ -z $link ]] || fail "包内包含符号链接：$link"
  [[ -f "$target/web/index.html" ]] || fail "缺 web/index.html"
  [[ -f "$target/web/404.html" ]] || fail "缺 web/404.html"
  [[ -d "$target/web/assets" ]] || fail "缺 web/assets"
  [[ -f "$target/release.json" ]] || fail "缺 release.json"
  grep -q "\"version\": \"$VERSION\"" "$target/release.json" || fail "release.json 版本不一致"
}

if (( DRY == 1 )); then
  mkdir "$TMP/release"
  extract_and_validate "$TMP/release"
  echo "DRY_RUN_OK"
  echo "version=$VERSION"
  echo "previous=$PREV_TARGET"
  echo "log=$LOG"
  exit 0
fi

mkdir "$RELEASE_DIR"
extract_and_validate "$RELEASE_DIR"
if [[ -d "$CURRENT_LINK/web/assets" ]]; then
  cp -a "$CURRENT_LINK/web/assets/." "$RELEASE_DIR/web/assets/"
fi
chown -R root:root "$RELEASE_DIR"
chmod -R a+rX "$RELEASE_DIR"
chmod 755 "$RELEASE_DIR/web"

NEXT="$WEB_ROOT/.current-$VERSION-$$"
rm -f "$NEXT"
ln -s "releases/$VERSION" "$NEXT"
mv -Tf "$NEXT" "$CURRENT_LINK"
SWITCHED=1
echo "已切换前端 current -> releases/$VERSION"

verify_site "$RELEASE_DIR" || fail "前端切换后验证失败"

echo "DEPLOY_OK"
echo "version=$VERSION"
echo "previous=$PREV_TARGET"
echo "release=$RELEASE_DIR"
echo "log=$LOG"
