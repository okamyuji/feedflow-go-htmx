# Phase0基盤と品質ゲート 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: feedflow-go-htmxのプロジェクト雛形と品質ゲート一式を整え、最小サーバで`bash scripts/quality-gate.sh`を緑にします。

Architecture: go-llm-agentの品質ゲート構成(quality-gate.sh、pre-commit、golangci-lint v2、gitleaks、govulncheck)を基線にし、feedflow向けに調整します。E2EはPhase9でPlaywrightを追加するため、Phase0のCIはpre-commitジョブのみとします。

Tech Stack: Go 1.25、golangci-lint v2.12.2、staticcheck、govulncheck、gitleaks 8.30.1、pre-commit。

前提: 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。`go version`が1.25系であることを確認してから始めます。

---

## Task 1: プロジェクト初期化

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/go.mod`
- Create: ディレクトリ構造

- [ ] Step 1: ディレクトリへ移動してgitを初期化する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx
git init
```
Expected: `Initialized empty Git repository`と表示されます。すでに初期化済みなら`Reinitialized`でも問題ありません。

- [ ] Step 2: ソースとスクリプトのディレクトリを作成する

Run:
```bash
mkdir -p cmd/feedflow internal/{domain,port,store,feed,service,poller,auth,handler,obs} web/templates web/static deploy/nginx e2e/playwright scripts/hooks .github/workflows
```
Expected: エラーなく完了します。

- [ ] Step 3: go.modを作成する

Create `go.mod`:
```
module github.com/okamyuji/feedflow-go-htmx

go 1.25.0
```
補足: golang.org/x/netとgolang.org/x/cryptoはそれぞれPhase3とPhase6で実際に使うときに`go get`で追加します。未使用のrequireは`go mod tidy`で消えるため、Phase0では足しません。

- [ ] Step 4: コミットする

```bash
git add go.mod
git commit -m "chore: go.modとディレクトリ構造を作成する"
```

---

## Task 2: gofmtフックとgolangci設定

Files:
- Create: `scripts/hooks/check_gofmt.sh`
- Create: `.golangci.yml`

- [ ] Step 1: gofmtチェックスクリプトを作成する

Create `scripts/hooks/check_gofmt.sh`:
```bash
#!/usr/bin/env bash
set -euo pipefail
diff=$(gofmt -l .)
if [[ -n "$diff" ]]; then
  echo "gofmtは次のファイルで差分を検出しました" >&2
  echo "$diff" >&2
  exit 1
fi
```

- [ ] Step 2: 実行権限を付与する

Run:
```bash
chmod +x scripts/hooks/check_gofmt.sh
```
Expected: エラーなく完了します。

- [ ] Step 3: golangci-lint設定を作成する

Create `.golangci.yml`:
```yaml
version: "2"
run:
  timeout: 5m
linters:
  default: none
  enable:
    - errcheck
    - govet
    - ineffassign
    - staticcheck
    - unused
    - gocritic
    - revive
    - gosec
    # リソースとメモリのリーク検出系
    - bodyclose      # net/http response.Body.Close漏れ
    - noctx          # context無しのnet/httpリクエスト
    - contextcheck   # 受け取ったcontextが下流に伝播されているか
    - prealloc
    - copyloopvar
    - misspell
  settings:
    revive:
      rules:
        - name: exported
          disabled: true
    gosec:
      excludes:
        - G104   # errcheckで別途検出する
        - G304   # 動的パス読み取りはdataディレクトリ配下に限定して防御する
    errcheck:
      exclude-functions:
        - fmt.Fprint
        - fmt.Fprintf
        - fmt.Fprintln
        - (io.Writer).Write
        - (*bytes.Buffer).Write
        - (*strings.Builder).WriteString
    gocritic:
      disabled-checks:
        - exitAfterDefer
        - hugeParam
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - gosec
          - errcheck
```
補足: rowserrcheckとsqlclosecheckはSQLを使わないため有効化しません。

- [ ] Step 4: コミットする

```bash
git add scripts/hooks/check_gofmt.sh .golangci.yml
git commit -m "chore: gofmtフックとgolangci-lint設定を追加する"
```

---

## Task 3: quality-gate.shとMakefile

Files:
- Create: `scripts/quality-gate.sh`
- Create: `Makefile`

- [ ] Step 1: quality-gate.shを作成する

Create `scripts/quality-gate.sh`:
```bash
#!/usr/bin/env bash
# quality-gate.sh
# pre-commitとCIから同一コマンドで呼び出す品質ゲート一式。
# gofmt / go vet / staticcheck / golangci-lint / govulncheck / go test(cover) / build / gitleaks。
# E2EはPlaywrightをCIの別ジョブで実行するためここには含めない。
set -euo pipefail

echo "==> gofmt"
./scripts/hooks/check_gofmt.sh

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
```

- [ ] Step 2: 実行権限を付与する

Run:
```bash
chmod +x scripts/quality-gate.sh
```
Expected: エラーなく完了します。

- [ ] Step 3: Makefileを作成する

Create `Makefile`:
```makefile
SHELL := /bin/bash
GO ?= go
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG := ./...

.PHONY: build test lint vuln quality secrets-scan precommit-install run fmt clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/feedflow ./cmd/feedflow

test:
	$(GO) test --count=1 --shuffle=on -race -cover $(PKG)

lint:
	$(GO) vet $(PKG)
	staticcheck $(PKG)
	golangci-lint run --timeout 5m $(PKG)

vuln:
	govulncheck $(PKG)

quality:
	./scripts/quality-gate.sh

secrets-scan:
	gitleaks detect --no-git --source . --redact --no-banner --config .gitleaks.toml

precommit-install:
	pre-commit install

fmt:
	$(GO) fmt $(PKG)

run: build
	./bin/feedflow

clean:
	rm -rf bin coverage.out
```

- [ ] Step 4: コミットする

```bash
git add scripts/quality-gate.sh Makefile
git commit -m "chore: quality-gate.shとMakefileを追加する"
```

---

## Task 4: gitleaksとpre-commitとgitignore

Files:
- Create: `.gitleaks.toml`
- Create: `.pre-commit-config.yaml`
- Create: `.gitignore`
- Create: `.env.example`

- [ ] Step 1: gitleaks設定を作成する

Create `.gitleaks.toml`:
```toml
title = "feedflow-go-htmx gitleaks config"

# gitleaks内蔵のデフォルトルール一式を継承する。
# これがないとallowlistのみが読まれてルール0件となり何も検出できない。
[extend]
useDefault = true

[allowlist]
description = """\
.env.exampleはマスク済みのサンプルなのでallowlist。
.envはローカル専用かつ.gitignoreで保護されておりgitに混入しないため
作業ツリーのno-git scanからは除外する。
"""
paths = [
  '''\.env\.example''',
  '''^\.env$''',
]
regexes = [
  '''\*\*\*MASKED\*\*\*''',
]
```

- [ ] Step 2: pre-commit設定を作成する

Create `.pre-commit-config.yaml`:
```yaml
# pre-commitとCIは同一のscripts/quality-gate.shを呼ぶ。
# entryにコロンとスペースを書くとpre-commit 4.xのYAMLパーサが
# mapping valueと誤認するためscriptsファイル化して呼び出す。
repos:
  - repo: local
    hooks:
      - id: quality-gate
        name: quality-gate (gofmt/vet/staticcheck/golangci/govulncheck/test/gitleaks)
        entry: bash scripts/quality-gate.sh
        language: system
        pass_filenames: false
        stages: [pre-commit]
```

- [ ] Step 3: gitignoreを作成する

Create `.gitignore`:
```
# ビルド成果物
/bin/
/dist/
coverage.out
coverage.html

# 実行時データ(JSON永続化、証明書)
/data/
*.pem
*.key
*.crt

# ローカル専用設定
.env

# ブレインストーミングの一時生成物
/.superpowers/

# NodeとPlaywright
node_modules/
/e2e/playwright/playwright-report/
/e2e/playwright/test-results/

# OSとエディタ
.DS_Store
```

- [ ] Step 4: env.exampleを作成する

Create `.env.example`:
```
# feedflow環境変数のサンプル。利用時はcp .env.example .envで複製し実値を入れる。
# .envは.gitignoreで保護される。

# サーバの待受アドレス
FEEDFLOW_ADDR=:8080

# JSON永続化のデータディレクトリ
FEEDFLOW_DATA_DIR=./data

# 公開時のベースURL(CookieのSecure判定や共有リンク生成に使う)
FEEDFLOW_BASE_URL=https://feedflow.example.com

# セッション署名鍵(Phase6で使用。openssl rand -base64 32などで生成した値を入れる)
FEEDFLOW_SESSION_KEY=***MASKED***
```

- [ ] Step 5: コミットする

```bash
git add .gitleaks.toml .pre-commit-config.yaml .gitignore .env.example
git commit -m "chore: gitleaksとpre-commitとgitignoreとenv.exampleを追加する"
```

---

## Task 5: verify-hardening.sh

Files:
- Create: `scripts/verify-hardening.sh`

- [ ] Step 1: verify-hardening.shを作成する

Create `scripts/verify-hardening.sh`:
```bash
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
```

- [ ] Step 2: 実行権限を付与する

Run:
```bash
chmod +x scripts/verify-hardening.sh
```
Expected: エラーなく完了します。

- [ ] Step 3: コミットする

```bash
git add scripts/verify-hardening.sh
git commit -m "chore: verify-hardening.shの最小版を追加する"
```

---

## Task 6: 最小サーバ(TDD)

Files:
- Test: `cmd/feedflow/main_test.go`
- Create: `cmd/feedflow/main.go`

- [ ] Step 1: 失敗するテストを書く

Create `cmd/feedflow/main_test.go`:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body got %q want %q", got, "ok")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
go test ./cmd/feedflow/ -run TestHealthz -v
```
Expected: コンパイルエラーで失敗します。`undefined: healthz`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `cmd/feedflow/main.go`:
```go
// Package main feedflowのエントリポイントを提供します。
package main

import (
	"log"
	"net/http"
	"time"
)

// versionビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("feedflow %s listening on %s", version, srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// healthz死活監視用のエンドポイントを返します。
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
go test ./cmd/feedflow/ -run TestHealthz -v
```
Expected: PASSします。

- [ ] Step 5: gofmtを適用する

Run:
```bash
gofmt -w cmd/feedflow/main.go cmd/feedflow/main_test.go
```
Expected: エラーなく完了します。

- [ ] Step 6: コミットする

```bash
git add cmd/feedflow/main.go cmd/feedflow/main_test.go
git commit -m "feat: 最小サーバとhealthzエンドポイントを追加する"
```

---

## Task 7: CIワークフロー

Files:
- Create: `.github/workflows/ci.yml`

- [ ] Step 1: CIワークフローを作成する

Create `.github/workflows/ci.yml`:
```yaml
# feedflow-go-htmx CI workflow
# untrusted user input(event.* 系)は一切利用しない。
# 実行に用いるのはリテラル文字列のenvと公式アクションだけとする。
name: CI

on:
  push:
    branches: ["**"]
  pull_request:
    branches: [main]

permissions:
  contents: read

env:
  FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: "true"

jobs:
  pre-commit:
    name: pre-commit (quality-gate + gitleaks)
    runs-on: ubuntu-latest
    env:
      GITLEAKS_VERSION: "8.30.1"
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0

      - name: Set up Go
        uses: actions/setup-go@v6
        with:
          go-version: "1.25"
          cache: true

      - name: Set up Python
        uses: actions/setup-python@v6
        with:
          python-version: "3.12"

      - name: Install staticcheck
        run: go install honnef.co/go/tools/cmd/staticcheck@latest

      - name: Install golangci-lint
        uses: golangci/golangci-lint-action@v9
        with:
          version: v2.12.2

      - name: Install govulncheck
        run: go install golang.org/x/vuln/cmd/govulncheck@v1.3.0

      - name: Install gitleaks
        env:
          VERSION: ${{ env.GITLEAKS_VERSION }}
        run: |
          set -euo pipefail
          curl -sSL "https://github.com/gitleaks/gitleaks/releases/download/v${VERSION}/gitleaks_${VERSION}_linux_x64.tar.gz" \
            | sudo tar -xz -C /usr/local/bin gitleaks

      - name: Install pre-commit
        run: pip install --upgrade pre-commit

      - name: Run pre-commit hooks (quality gate)
        run: pre-commit run --all-files --show-diff-on-failure --color always

      - name: Run hardening verification
        run: bash scripts/verify-hardening.sh
```
補足: Playwrightを使うe2eジョブはPhase9でこのファイルに追加します。Phase0時点でe2eジョブを置くと実体が無く失敗するため入れません。

- [ ] Step 2: コミットする

```bash
git add .github/workflows/ci.yml
git commit -m "ci: pre-commitと品質ゲートを実行するCIを追加する"
```

---

## Task 8: 品質ゲートの緑化とpre-commitインストール

- [ ] Step 1: ツールがそろっているか確認する

Run:
```bash
command -v staticcheck golangci-lint govulncheck gitleaks pre-commit
```
Expected: いずれもパスが表示されます。未インストールのものがあれば次でインストールします。

- [ ] Step 2: 不足ツールをインストールする(未導入のもののみ)

Run:
```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
go install golang.org/x/vuln/cmd/govulncheck@v1.3.0
```
golangci-lintとgitleaksとpre-commitはOSのパッケージマネージャか公式手順で導入します。macOSなら`brew install golangci-lint gitleaks pre-commit`が使えます。
Expected: エラーなく完了します。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
bash scripts/quality-gate.sh
```
Expected: `all quality checks passed`で終わります。途中で失敗したら、表示されたlintやvetの指摘を修正してから再実行します。

- [ ] Step 4: pre-commitフックをインストールする

Run:
```bash
pre-commit install
```
Expected: `pre-commit installed at .git/hooks/pre-commit`と表示されます。

- [ ] Step 5: pre-commitを全ファイルに対して実行する

Run:
```bash
pre-commit run --all-files
```
Expected: quality-gateフックがPassedになります。

- [ ] Step 6: ここまでをコミットする(フック経由で品質ゲートが走る)

```bash
git add -A
git commit -m "chore: 品質ゲートを緑化しpre-commitを有効化する"
```
Expected: コミット時にquality-gateが走り、緑のままコミットされます。

---

## Task 9: README

Files:
- Create: `README.md`

- [ ] Step 1: READMEを作成する

Create `README.md`:
```markdown
# feedflow-go-htmx

Web上で動作する個人専用のRSS Readerです。Goの標準ライブラリ中心とHTMXとAlpine.jsで実装します。

## 必要なもの

- Go 1.25以降
- golangci-lint v2.12.2、staticcheck、govulncheck、gitleaks、pre-commit
- Node.js 24(E2Eを実行する場合)

## セットアップ

```bash
cp .env.example .env
# .envを編集して実値を入れる
pre-commit install
```

## 開発コマンド

```bash
make build     # bin/feedflowをビルドする
make test      # テストを実行する
make quality   # 品質ゲート一式を実行する
make run       # ビルドして起動する
```

## 品質ゲート

`scripts/quality-gate.sh`をpre-commitとCIから同一コマンドで実行します。gofmt、go vet、staticcheck、golangci-lint、govulncheck、go test、gitleaksを通します。

## ドキュメント

- 設計書`docs/superpowers/specs/2026-05-29-feedflow-design.md`
- 実装計画`docs/superpowers/plans/`
```

- [ ] Step 2: コミットする

```bash
git add README.md
git commit -m "docs: READMEを追加する"
```

---

## Phase0完了条件

- [ ] `bash scripts/quality-gate.sh`が`all quality checks passed`で終わる
- [ ] `go test ./...`が通る
- [ ] `pre-commit run --all-files`がPassedになる
- [ ] コミットが規約に沿って積まれている
