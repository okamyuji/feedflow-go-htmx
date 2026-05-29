#!/usr/bin/env bash
# verify-hardening.sh
# セキュリティハードニングが破壊シナリオを拒否することをローカルで再現確認する。
# Phase0では最小構成(全テストとビルド)で始め、Phase6とPhase7で
# SSRF拒否、初回セットアップ無効化、CSRF検証などのシナリオを追加する。
# 依存はgoとbashのみ。固有ホストや個人パスへの依存はない。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

YELLOW='\033[33m'
GREEN='\033[32m'
RED='\033[31m'
NC='\033[0m'

pass=0
fail=0

check() {
  local name="$1"
  shift
  printf "${YELLOW}>>> %s${NC}\n" "$name"
  if "$@"; then
    printf "${GREEN}    PASS${NC}\n"
    pass=$((pass+1))
  else
    printf "${RED}    FAIL${NC}\n"
    fail=$((fail+1))
  fi
}

command -v go >/dev/null 2>&1 || { echo "go not found"; exit 2; }

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

check "unit tests (go test -race ./...)" bash -c "go test -race -count=1 ./... >/dev/null"
check "build feedflow binary" bash -c "go build -o '$WORK/feedflow' ./cmd/feedflow"

echo
echo "----------------------------------------"
printf "passed: ${GREEN}%d${NC}  failed: ${RED}%d${NC}\n" "$pass" "$fail"
echo "----------------------------------------"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
