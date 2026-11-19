#!/usr/bin/env bash
set -euo pipefail

fail() { echo "构建失败：$*" >&2; exit 1; }

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

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$@"
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$@"
  else
    fail "缺少 sha256sum/shasum"
  fi
}

ROOT=$(find_root) || fail "未找到 Laoshi 仓库根目录"
SERVER_DIR="$ROOT/server"
command -v go >/dev/null 2>&1 || fail "未找到 go。需要 Go 1.23+；安装后重试（macOS 可用 brew install go）"
command -v tar >/dev/null 2>&1 || fail "缺少 tar"
command -v file >/dev/null 2>&1 || fail "缺少 file"

GO_VERSION=$(go env GOVERSION 2>/dev/null || true)
[[ $GO_VERSION == go* ]] || fail "无法读取 Go 版本"
GO_NUM=${GO_VERSION#go}
GO_MAJOR=${GO_NUM%%.*}
GO_REST=${GO_NUM#*.}
GO_MINOR=${GO_REST%%.*}
if (( GO_MAJOR < 1 || (GO_MAJOR == 1 && GO_MINOR < 23) )); then
  fail "Go 版本过低：$GO_VERSION（需要 1.23+）"
fi

APP_VERSION=$(awk -F'"' '/"version":/ { print $4; exit }' "$ROOT/package.json")
[[ -n $APP_VERSION ]] || fail "无法从 package.json 读取版本"

COMMIT=$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo nogit)
DIRTY=false
if [[ -n $(git -C "$ROOT" status --porcelain 2>/dev/null) ]]; then
  DIRTY=true
  echo "警告：工作区存在未提交改动，release.json 将标记 dirty=true。" >&2
fi

SHORT_SHA=$(git -C "$ROOT" rev-parse --short=7 HEAD 2>/dev/null || echo nogit)
VERSION=${RELEASE_ID:-$(date -u +%Y%m%dT%H%M%SZ)-$SHORT_SHA}
[[ $VERSION =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,95}$ ]] || fail "版本号不安全：$VERSION"

BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/laoshi-backend.XXXXXX")
trap 'rm -rf "$BUILD_DIR"' EXIT
PKG_NAME="laoshi-backend-$VERSION"
PKG_DIR="$BUILD_DIR/$PKG_NAME"
mkdir -p "$PKG_DIR/db/migrations" "$PKG_DIR/deploy"

echo "构建 $PKG_NAME（linux/amd64，$GO_VERSION）..."
(
  cd "$SERVER_DIR"
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=local \
    go build -trimpath -buildvcs=false -mod=readonly -o "$PKG_DIR/laoshi-api" ./cmd/api
)

FILE_INFO=$(file "$PKG_DIR/laoshi-api")
case "$FILE_INFO" in
  *"ELF 64-bit LSB"*x86-64*) ;;
  *) fail "产物不是 linux/amd64 ELF：$FILE_INFO" ;;
esac
BUILD_INFO=$(go version -m "$PKG_DIR/laoshi-api")
case "$BUILD_INFO" in
  *GOOS=linux*) ;;
  *) fail "产物 GOOS 不是 linux" ;;
esac
case "$BUILD_INFO" in
  *GOARCH=amd64*) ;;
  *) fail "产物 GOARCH 不是 amd64" ;;
esac

shopt -s nullglob
MIGRATIONS=("$SERVER_DIR"/db/migrations/*.sql)
shopt -u nullglob
(( ${#MIGRATIONS[@]} > 0 )) || fail "没有找到迁移 SQL"
cp -p "${MIGRATIONS[@]}" "$PKG_DIR/db/migrations/"
cp -p "$SERVER_DIR/api/openapi.yaml" "$PKG_DIR/openapi.yaml"
cp -p "$ROOT/deploy/laoshi-api.service" "$PKG_DIR/deploy/laoshi-api.service"
cp -p "$ROOT/deploy/laoshi-api.env.example" "$PKG_DIR/deploy/laoshi-api.env.example"

PACKAGED_AT=$(date -u +%Y-%m-%dT%H:%M:%SZ)
cat > "$PKG_DIR/release.json" <<EOF
{
  "version": "$VERSION",
  "appVersion": "$APP_VERSION",
  "packagedAt": "$PACKAGED_AT",
  "goVersion": "$GO_VERSION",
  "goos": "linux",
  "goarch": "amd64",
  "cgoEnabled": false,
  "entrypoint": "cmd/api",
  "migration": "embedded-goose",
  "migrations": ${#MIGRATIONS[@]},
  "git": {
    "commit": "$COMMIT",
    "dirty": $DIRTY
  }
}
EOF

find "$PKG_DIR" -name '.DS_Store' -delete
(
  cd "$PKG_DIR"
  sha256 laoshi-api db/migrations/*.sql openapi.yaml \
    deploy/laoshi-api.service deploy/laoshi-api.env.example > SHA256SUMS
)

OUT="$ROOT/artifacts/backend"
[[ ! -L $OUT ]] || fail "artifacts/backend 不允许符号链接"
mkdir -p "$OUT"
ARCHIVE="$OUT/$PKG_NAME-linux-amd64.tar.gz"
CHECKSUM="$ARCHIVE.sha256"
[[ ! -e $ARCHIVE ]] || fail "已存在同版本归档：$ARCHIVE"
[[ ! -e $CHECKSUM ]] || fail "已存在同版本校验和：$CHECKSUM"

TMP_ARCHIVE="$OUT/.$PKG_NAME-$$.tar.gz"
trap 'rm -rf "$BUILD_DIR"; rm -f "$TMP_ARCHIVE"' EXIT
(
  cd "$BUILD_DIR"
  COPYFILE_DISABLE=1 tar -czf "$TMP_ARCHIVE" "$PKG_NAME"
)
tar -tzf "$TMP_ARCHIVE" > "$BUILD_DIR/list.txt"
grep -qx "$PKG_NAME/laoshi-api" "$BUILD_DIR/list.txt" || fail "归档缺少 laoshi-api"
[[ ! -e $ARCHIVE ]] || fail "归档已存在"
mv "$TMP_ARCHIVE" "$ARCHIVE"
(
  cd "$OUT"
  sha256 "$(basename "$ARCHIVE")" > "$(basename "$CHECKSUM")"
)

SIZE=$(wc -c < "$ARCHIVE")
SIZE=${SIZE// /}
echo "构建完成：$APP_VERSION / $VERSION"
echo "VERSION=$VERSION"
echo "ARTIFACT=$ARCHIVE"
echo "CHECKSUM=$CHECKSUM"
echo "SIZE=$SIZE"
