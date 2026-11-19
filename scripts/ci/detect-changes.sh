#!/usr/bin/env bash
set -euo pipefail

fail() { echo "变更检测失败：$*" >&2; exit 1; }

emit() {
  local key=$1 value=$2
  if [[ -n ${GITHUB_OUTPUT:-} ]]; then
    printf '%s=%s\n' "$key" "$value" >> "$GITHUB_OUTPUT"
  fi
  printf '%s=%s\n' "$key" "$value"
}

BASE_REF=${1:-}
HEAD_REF=${2:-HEAD}
FORCE=${FORCE:-auto}

case "$FORCE" in
  auto|api|web|both) ;;
  *) fail "FORCE 只能是 auto/api/web/both，实际：$FORCE" ;;
esac

if [[ $FORCE != auto ]]; then
  case "$FORCE" in
    api) emit api true; emit web false ;;
    web) emit api false; emit web true ;;
    both) emit api true; emit web true ;;
  esac
  emit config false
  exit 0
fi

if [[ -z $BASE_REF ]]; then
  BASE_REF=$(git describe --tags --abbrev=0 "$HEAD_REF^" 2>/dev/null || true)
fi

if [[ -z $BASE_REF ]]; then
  echo "没有找到上一个 tag，按前后端都变更处理。"
  emit api true
  emit web true
  emit config false
  exit 0
fi

git rev-parse --verify "$BASE_REF^{commit}" >/dev/null 2>&1 || fail "base 不存在：$BASE_REF"
git rev-parse --verify "$HEAD_REF^{commit}" >/dev/null 2>&1 || fail "head 不存在：$HEAD_REF"

api=false
web=false
config=false

while IFS= read -r file; do
  [[ -n $file ]] || continue
  case "$file" in
    server/*) api=true ;;
    scripts/ci/*) ;;
    src/*|content/*|public/*|index.html|vite.config.ts|tsconfig.json|package.json|package-lock.json|scripts/*) web=true ;;
    deploy/nginx/*|deploy/laoshi-api.service|deploy/laoshi-api.env.example) config=true ;;
  esac
done < <(git diff --name-only "$BASE_REF..$HEAD_REF")

echo "对比范围：$BASE_REF..$HEAD_REF"
emit api "$api"
emit web "$web"
emit config "$config"
