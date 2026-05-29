#!/usr/bin/env bash
# issue-tls-cert.sh
# Let's Encryptのサーバ証明書をhttp-01チャレンジで取得します。
# 前提として80番が到達可能でDOMAINがEC2のEIPを指していることが必要です。
set -euo pipefail

DOMAIN="${1:?usage: issue-tls-cert.sh <domain> <email>}"
EMAIL="${2:?usage: issue-tls-cert.sh <domain> <email>}"
WEBROOT="/var/www/certbot"

sudo mkdir -p "$WEBROOT"

# certbotをDockerで実行しwebroot方式で取得します。
sudo docker run --rm \
  -v /etc/letsencrypt:/etc/letsencrypt \
  -v "$WEBROOT:$WEBROOT" \
  certbot/certbot certonly \
  --webroot -w "$WEBROOT" \
  -d "$DOMAIN" \
  --email "$EMAIL" \
  --agree-tos --no-eff-email --non-interactive

echo "発行済み証明書のパスは次のとおりです /etc/letsencrypt/live/$DOMAIN/fullchain.pem"
echo "次のコマンドでnginxをリロードして反映します sudo docker compose exec nginx nginx -s reload"
