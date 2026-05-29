#!/usr/bin/env bash
# make-mtls-ca.sh
# mTLSクライアント証明書を発行するための自前ローカルCAを作成します。
# CA秘密鍵はリポジトリへ置かずホストの保護ディレクトリにだけ保管します。
set -euo pipefail

OUT_DIR="${1:-/etc/feedflow/mtls}"
DAYS="${2:-3650}"

sudo mkdir -p "$OUT_DIR"
sudo chmod 700 "$OUT_DIR"

# CA秘密鍵を生成します。
sudo openssl genrsa -out "$OUT_DIR/ca.key" 4096
sudo chmod 600 "$OUT_DIR/ca.key"

# 自己署名のCA証明書を生成します。
sudo openssl req -x509 -new -nodes \
  -key "$OUT_DIR/ca.key" \
  -sha256 -days "$DAYS" \
  -subj "/CN=feedflow-mtls-ca/O=feedflow" \
  -out "$OUT_DIR/ca.crt"

echo "CA証明書をnginxのssl_client_certificateに指定します パスは $OUT_DIR/ca.crt"
echo "CA秘密鍵は絶対に配布しません パスは $OUT_DIR/ca.key"
