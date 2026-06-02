#!/usr/bin/env bash
# quality-gate.sh
# pre-commitとCIから同一コマンドで呼び出す品質ゲート一式。
# gofmt / go fix / go vet / staticcheck / golangci-lint / govulncheck / go test(cover) / build / gitleaks。
# E2EはPlaywrightをCIの別ジョブで実行するためここには含めない。
set -euo pipefail

echo "==> gofmt"
./scripts/hooks/check_gofmt.sh

echo "==> go fix -diff (近代化の適用漏れ検査)"
# -diffは書き換えず差分のみ出力し、差分が非空なら非ゼロ終了する(gofmt -lと同じ非破壊方式)。
# set -e下では非ゼロ終了でその場で死ぬため、|| trueで受けて差分を捕捉し、自前のメッセージで失敗させる。
fix_diff="$(go fix -diff ./... || true)"
if [[ -n "$fix_diff" ]]; then
  echo "ERROR: go fix で近代化できる箇所が残っています。'go fix ./...' を実行してコミットしてください。" >&2
  echo "$fix_diff" >&2
  exit 2
fi

echo "==> go vet"
go vet ./...

echo "==> staticcheck"
staticcheck ./...

echo "==> golangci-lint"
golangci-lint run --timeout 5m ./...

echo "==> govulncheck"
govulncheck ./...

echo "==> go test (count=1 shuffle=on race cover)"
go test --count=1 --shuffle=on -race -coverprofile=coverage.out -covermode=atomic ./...

echo "==> coverage summary (目標80%、未達でもfailしない)"
go tool cover -func=coverage.out | tail -n 1

echo "==> release build smoke (go build -o bin/feedflow)"
mkdir -p bin
go build -o bin/feedflow ./cmd/feedflow

echo "==> staged-secret-files-guard"
staged_sensitive=$(git diff --cached --name-only 2>/dev/null | grep -E '^(\.env|config\.yaml)$' || true)
if [ -n "$staged_sensitive" ]; then
  echo "ERROR: 以下のファイルはコミットしてはいけません(ローカル専用)" >&2
  echo "$staged_sensitive" >&2
  echo "  git reset HEAD <file> でstagingから外してください" >&2
  exit 2
fi

echo "==> gitleaks (detect --no-git: 作業ツリーとstagedをスキャンする)"
if command -v gitleaks >/dev/null 2>&1; then
  gitleaks detect --no-git --source . --redact --no-banner --config .gitleaks.toml
else
  echo "  (gitleaks未インストールのためスキップ。CIはインストールします)"
fi

echo "all quality checks passed"
