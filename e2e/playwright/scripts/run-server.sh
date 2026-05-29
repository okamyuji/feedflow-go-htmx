#!/usr/bin/env bash
# run-server.sh
# PlaywrightのwebServerから呼び出し、隔離したデータディレクトリでfeedflowを起動します。
# 毎回データを作り直すため、初回セットアップ画面から始まる決定的な状態になります。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

DATA_DIR="$ROOT/e2e/playwright/data-e2e"
BIN="$ROOT/e2e/playwright/.cache/feedflow-e2e"

rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR"
mkdir -p "$(dirname "$BIN")"

go build -o "$BIN" ./cmd/feedflow

export FEEDFLOW_ADDR=":8099"
export FEEDFLOW_DATA_DIR="$DATA_DIR"
export FEEDFLOW_BASE_URL="http://127.0.0.1:8099"
export FEEDFLOW_SESSION_KEY="e2e-test-session-key-not-secret-0123456789"
# E2Eはスイートで多数ログインするため、ログイン試行のレート制限を実質無効化します。
export FEEDFLOW_LOGIN_BURST="100000"
# E2Eのテスト用フィードサーバはループバックにあるため、SSRF対策のプライベート宛拒否を解除します。
export FEEDFLOW_ALLOW_PRIVATE_FETCH="1"

exec "$BIN"
