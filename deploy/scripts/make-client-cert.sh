#!/usr/bin/env bash
# make-client-cert.sh
# ローカルCAでクライアント証明書を発行しブラウザ取り込み用のPKCS#12を生成します。
# 出力する.p12とパスフレーズだけを所有者へ安全に配布します。
set -euo pipefail

CA_DIR="${1:-/etc/feedflow/mtls}"
CLIENT_NAME="${2:?usage: make-client-cert.sh <ca_dir> <client_name> <out_dir>}"
OUT_DIR="${3:-./client-certs}"
DAYS="${4:-825}"

mkdir -p "$OUT_DIR"

# クライアント秘密鍵を生成します。
openssl genrsa -out "$OUT_DIR/$CLIENT_NAME.key" 2048

# CSRを生成します。
openssl req -new \
  -key "$OUT_DIR/$CLIENT_NAME.key" \
  -subj "/CN=$CLIENT_NAME/O=feedflow-client" \
  -out "$OUT_DIR/$CLIENT_NAME.csr"

# CAで署名しクライアント証明書を発行します。
sudo openssl x509 -req \
  -in "$OUT_DIR/$CLIENT_NAME.csr" \
  -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" \
  -CAcreateserial \
  -days "$DAYS" -sha256 \
  -out "$OUT_DIR/$CLIENT_NAME.crt"

# ブラウザ取り込み用のPKCS#12を生成します。取り込みパスフレーズを対話入力します。
openssl pkcs12 -export \
  -inkey "$OUT_DIR/$CLIENT_NAME.key" \
  -in "$OUT_DIR/$CLIENT_NAME.crt" \
  -certfile "$CA_DIR/ca.crt" \
  -name "feedflow client $CLIENT_NAME" \
  -out "$OUT_DIR/$CLIENT_NAME.p12"

echo "配布物のパスは次のとおりです $OUT_DIR/$CLIENT_NAME.p12"
echo "取り込み手順はブラウザの証明書管理から個人証明書として.p12をインポートしパスフレーズを入力します"
echo "csrとkeyは配布せずローカルで破棄してよいです"
