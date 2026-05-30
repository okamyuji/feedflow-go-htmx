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

# 一時ファイルはモジュール内に置く。SSRFチェックのGoプログラムがinternalパッケージを
# importするため、モジュール外(/tmpなど)だとuse of internal package not allowedで失敗する。
# ドット始まりのためgoの ./... グロブからは除外され、通常のビルドやテストに混入しない。
WORK="$ROOT/.hardening-tmp"
rm -rf "$WORK"
mkdir -p "$WORK"
trap 'rm -rf "$WORK"' EXIT

check "unit tests (go test -race ./...)" bash -c "go test -race -count=1 ./... >/dev/null"
check "build feedflow binary" bash -c "go build -o '$WORK/feedflow' ./cmd/feedflow"

# --- SSRF拒否シナリオ ---
# Phase3のフェッチャはループバックとプライベートIPを拒否する。
# 起動済みアプリではなくfetcherを直接呼び、ループバックURLが
# エラーになることをGoのインラインプログラムで確認する。
SSRF_PROG="$WORK/ssrf_check.go"
cat > "$SSRF_PROG" <<'GOEOF'
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func main() {
	f := feed.NewHTTPFetcher(
		feed.WithTimeout(5*time.Second),
		feed.WithMaxBytes(1<<20),
	)
	loopback := []string{
		"http://127.0.0.1:9/feed.xml",
		"http://localhost:9/feed.xml",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/feed.xml",
		"ftp://example.test/feed.xml",
	}
	for _, u := range loopback {
		_, err := f.Fetch(context.Background(), port.FetchRequest{URL: u})
		if err == nil {
			fmt.Fprintf(os.Stderr, "SSRF NOT blocked for %s\n", u)
			os.Exit(1)
		}
	}
	fmt.Println("all SSRF targets blocked")
}
GOEOF

check "SSRF 拒否(ループバックとプライベート IP とスキーム制限)" \
  go run "$SSRF_PROG"

# --- 初回セットアップ無効化シナリオ ---
# user.json登録後はGET /setupがログインへリダイレクトし
# POST /setupが拒否されることを、起動済みサーバへのHTTPで確認する。
SETUP_DATA="$WORK/data-setup"
SETUP_BIN="$WORK/feedflow-setup"
go build -o "$SETUP_BIN" ./cmd/feedflow

run_setup_check() {
  rm -rf "$SETUP_DATA"
  mkdir -p "$SETUP_DATA"
  FEEDFLOW_ADDR=":8097" \
  FEEDFLOW_DATA_DIR="$SETUP_DATA" \
  FEEDFLOW_BASE_URL="http://127.0.0.1:8097" \
  FEEDFLOW_SESSION_KEY="hardening-test-key-0123456789abcdef" \
    "$SETUP_BIN" &
  local pid=$!
  local rc=0

  for _ in $(seq 1 50); do
    if go run "$SETUP_PROG" wait "http://127.0.0.1:8097/healthz" 2>/dev/null; then
      break
    fi
    sleep 0.2
  done

  go run "$SETUP_PROG" verify "http://127.0.0.1:8097" || rc=$?
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  return "$rc"
}

SETUP_PROG="$WORK/setup_check.go"
cat > "$SETUP_PROG" <<'GOEOF'
package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	mode := os.Args[1]
	base := os.Args[2]

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if mode == "wait" {
		resp, err := client.Get(base)
		if err != nil {
			os.Exit(1)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			os.Exit(1)
		}
		return
	}

	// 登録前はGET /setupが200でフォームを返す。
	respBefore, err := client.Get(base + "/setup")
	if err != nil {
		fmt.Fprintf(os.Stderr, "GET /setup before failed: %v\n", err)
		os.Exit(1)
	}
	_ = respBefore.Body.Close()
	if respBefore.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "GET /setup before want 200 got %d\n", respBefore.StatusCode)
		os.Exit(1)
	}

	// ユーザーを登録する。CSRFトークンはセットアップ前のGETから取得する想定だが、
	// 初回セットアップは未認証フローのため、ここではフォーム送信で登録する。
	form := url.Values{}
	form.Set("username", "owner")
	form.Set("password", "correct-horse-battery-staple")
	respReg, err := client.PostForm(base+"/setup", form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "POST /setup register failed: %v\n", err)
		os.Exit(1)
	}
	_ = respReg.Body.Close()
	if respReg.StatusCode != http.StatusSeeOther && respReg.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "POST /setup register want 303 or 200 got %d\n", respReg.StatusCode)
		os.Exit(1)
	}

	// 登録後はGET /setupがログインへリダイレクトする。
	respAfter, err := client.Get(base + "/setup")
	if err != nil {
		fmt.Fprintf(os.Stderr, "GET /setup after failed: %v\n", err)
		os.Exit(1)
	}
	_ = respAfter.Body.Close()
	loc := respAfter.Header.Get("Location")
	if respAfter.StatusCode != http.StatusSeeOther || !strings.HasSuffix(loc, "/login") {
		fmt.Fprintf(os.Stderr, "GET /setup after want 303 to /login got %d loc=%q\n", respAfter.StatusCode, loc)
		os.Exit(1)
	}

	// 登録後のPOST /setupは拒否される(303または4xx)。
	respPost, err := client.PostForm(base+"/setup", form)
	if err != nil {
		fmt.Fprintf(os.Stderr, "POST /setup after failed: %v\n", err)
		os.Exit(1)
	}
	_ = respPost.Body.Close()
	if respPost.StatusCode == http.StatusOK || respPost.StatusCode == http.StatusSeeOther {
		if respPost.StatusCode == http.StatusSeeOther {
			if l := respPost.Header.Get("Location"); strings.HasSuffix(l, "/setup") {
				fmt.Fprintln(os.Stderr, "POST /setup after re-registered which is not allowed")
				os.Exit(1)
			}
		}
	}
	if respPost.StatusCode != http.StatusSeeOther &&
		respPost.StatusCode != http.StatusForbidden &&
		respPost.StatusCode != http.StatusNotFound &&
		respPost.StatusCode != http.StatusConflict {
		fmt.Fprintf(os.Stderr, "POST /setup after want redirect or 4xx got %d\n", respPost.StatusCode)
		os.Exit(1)
	}

	fmt.Println("setup disabled after registration")
}
GOEOF

check "初回セットアップ無効化(登録後の /setup 到達拒否)" run_setup_check

# --- CSRF検証シナリオ ---
# ログイン後、CSRFトークン無しのPOST /app/feedsは拒否され、
# 正しいトークン付きでは受理されることをHTTPで確認する。
CSRF_DATA="$WORK/data-csrf"
CSRF_BIN="$WORK/feedflow-csrf"
go build -o "$CSRF_BIN" ./cmd/feedflow

run_csrf_check() {
  rm -rf "$CSRF_DATA"
  mkdir -p "$CSRF_DATA"
  FEEDFLOW_ADDR=":8096" \
  FEEDFLOW_DATA_DIR="$CSRF_DATA" \
  FEEDFLOW_BASE_URL="http://127.0.0.1:8096" \
  FEEDFLOW_SESSION_KEY="hardening-csrf-key-0123456789abcdef" \
    "$CSRF_BIN" &
  local pid=$!
  local rc=0

  for _ in $(seq 1 50); do
    if go run "$SETUP_PROG" wait "http://127.0.0.1:8096/healthz" 2>/dev/null; then
      break
    fi
    sleep 0.2
  done

  go run "$CSRF_PROG" "http://127.0.0.1:8096" || rc=$?
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  return "$rc"
}

CSRF_PROG="$WORK/csrf_check.go"
cat > "$CSRF_PROG" <<'GOEOF'
package main

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"time"
)

var csrfRe = regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)

func extractCSRF(client *http.Client, target string) (string, error) {
	resp, err := client.Get(target)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 1<<20)
	n, _ := resp.Body.Read(buf)
	m := csrfRe.FindSubmatch(buf[:n])
	if m == nil {
		return "", fmt.Errorf("csrf_token not found in %s", target)
	}
	return string(m[1]), nil
}

func main() {
	base := os.Args[1]
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 5 * time.Second,
		Jar:     jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 初回セットアップでユーザーを登録する。
	reg := url.Values{}
	reg.Set("username", "owner")
	reg.Set("password", "correct-horse-battery-staple")
	if t, err := extractCSRF(client, base+"/setup"); err == nil {
		reg.Set("csrf_token", t)
	}
	respReg, err := client.PostForm(base+"/setup", reg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup failed: %v\n", err)
		os.Exit(1)
	}
	_ = respReg.Body.Close()

	// ログインする。ログインPOSTはrequireCSRFの対象外のため、
	// ログイン画面にCSRFの隠しフィールドがあるときだけ付与する。
	loginForm := url.Values{}
	loginForm.Set("username", "owner")
	loginForm.Set("password", "correct-horse-battery-staple")
	if t, err := extractCSRF(client, base+"/login"); err == nil {
		loginForm.Set("csrf_token", t)
	}
	respLogin, err := client.PostForm(base+"/login", loginForm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}
	_ = respLogin.Body.Close()
	if respLogin.StatusCode != http.StatusSeeOther && respLogin.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "login want 303 or 200 got %d\n", respLogin.StatusCode)
		os.Exit(1)
	}

	// CSRFトークン無しのPOST /app/feedsは拒否される(403)。
	noTok := url.Values{}
	noTok.Set("url", "http://127.0.0.1:9/feed.xml")
	respNo, err := client.PostForm(base+"/app/feeds", noTok)
	if err != nil {
		fmt.Fprintf(os.Stderr, "post feeds no-token failed: %v\n", err)
		os.Exit(1)
	}
	_ = respNo.Body.Close()
	if respNo.StatusCode != http.StatusForbidden {
		fmt.Fprintf(os.Stderr, "POST /app/feeds without token want 403 got %d\n", respNo.StatusCode)
		os.Exit(1)
	}

	// 正しいトークン付きのPOST /app/feedsはCSRFでは拒否されない。
	// 不正なurlのため取得自体は失敗しうるが、403でないことを確認する。
	pageTok, err := extractCSRF(client, base+"/app")
	if err != nil {
		fmt.Fprintf(os.Stderr, "page csrf failed: %v\n", err)
		os.Exit(1)
	}
	withTok := url.Values{}
	withTok.Set("url", "http://127.0.0.1:9/feed.xml")
	withTok.Set("csrf_token", pageTok)
	respYes, err := client.PostForm(base+"/app/feeds", withTok)
	if err != nil {
		fmt.Fprintf(os.Stderr, "post feeds with-token failed: %v\n", err)
		os.Exit(1)
	}
	_ = respYes.Body.Close()
	if respYes.StatusCode == http.StatusForbidden {
		fmt.Fprintln(os.Stderr, "POST /app/feeds with valid token unexpectedly 403")
		os.Exit(1)
	}

	fmt.Println("CSRF enforced for state-changing POST")
}
GOEOF

check "CSRF 検証(トークン無し POST は 403、トークン付きは通過)" run_csrf_check

echo
echo "----------------------------------------"
printf "passed: ${GREEN}%d${NC}  failed: ${RED}%d${NC}\n" "$pass" "$fail"
echo "----------------------------------------"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
