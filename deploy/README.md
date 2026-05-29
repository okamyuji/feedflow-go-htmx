# feedflow デプロイ手順

単一EC2(ARMのt4g系)とEBS、nginxコンテナとGoアプリコンテナの同居でfeedflowを公開します。ALBとNLBは使いません。前段のnginxでTLS終端とmTLSを行い、所有者だけがアクセスできるようにします。

## 構成図

```
[ブラウザとクライアント証明書]
        | 443でTLSとmTLS
        v
[EC2 t4g] -- compose --+-- nginx(443終端 / mTLS検証 / リバースプロキシ)
                       |        | 内部ネットワーク8080
                       +-- app(feedflow単一バイナリ / embed同梱)
                                | /data
                       [EBSマウント /mnt/feedflow-data]
```

## 1 EC2とEBSの準備

1. ARMのt4gインスタンスをAmazon Linux 2023のARM版で起動します。インスタンスタイプはt4g.smallなどを選びます
2. EIPを割り当てて固定し、DNSのAレコードを`feedflow.example.com`からこのEIPへ向けます
3. データ用のEBSボリュームを作成しインスタンスへアタッチします
4. EBSをフォーマットして`/mnt/feedflow-data`へマウントします

```bash
# デバイス名は環境で異なるためlsblkで確認します
lsblk
sudo mkfs -t ext4 /dev/nvme1n1
sudo mkdir -p /mnt/feedflow-data
sudo mount /dev/nvme1n1 /mnt/feedflow-data
# 再起動後も維持するためfstabへ追記します
echo "/dev/nvme1n1 /mnt/feedflow-data ext4 defaults,nofail 0 2" | sudo tee -a /etc/fstab
```

## 2 Security Group

`deploy/security-group.json`の内容で受信規則を設定します。443は全世界へ開きますがmTLSで証明書なしを拒否します。80はLet's Encryptのhttp-01チャレンジと443へのリダイレクトのみに使います。SSHは運用元のIPの/32だけに限定し、JSON内の`203.0.113.10/32`を自分のグローバルIPへ書き換えます。

```bash
MYIP="$(curl -fsS https://checkip.amazonaws.com)"
echo "SSHを許可する自分のIPを表示します $MYIP/32 をsecurity-group.jsonの203.0.113.10/32と置換します"
```

## 3 Dockerとcomposeの導入

```bash
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
# composeプラグインを導入します
sudo mkdir -p /usr/libexec/docker/cli-plugins
ARCH="$(uname -m)"
sudo curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${ARCH}" \
  -o /usr/libexec/docker/cli-plugins/docker-compose
sudo chmod +x /usr/libexec/docker/cli-plugins/docker-compose
docker compose version
```

## 4 ソース配置と環境変数

```bash
git clone https://github.com/okamyuji/feedflow-go-htmx.git
cd feedflow-go-htmx
cp .env.example .env
# .envを編集します。最低でも次を設定します
#   FEEDFLOW_BASE_URL=https://feedflow.example.com
#   FEEDFLOW_SESSION_KEY=$(openssl rand -base64 32)
export FEEDFLOW_BASE_URL=https://feedflow.example.com
export FEEDFLOW_SESSION_KEY="$(openssl rand -base64 32)"
```

## 5 mTLSのCAとクライアント証明書

```bash
# CAを作成します。出力は/etc/feedflow/mtlsです
sudo bash deploy/scripts/make-mtls-ca.sh /etc/feedflow/mtls
# 自分用のクライアント証明書を発行します
bash deploy/scripts/make-client-cert.sh /etc/feedflow/mtls okamyuji ./client-certs
# 生成された./client-certs/okamyuji.p12をローカル端末へscpで取得しブラウザへ取り込みます
```

クライアント証明書の取り込み手順です。

- macOSのChromeとSafariはキーチェーンアクセスへ.p12をインポートし、発行時のパスフレーズを入力します
- Firefoxは設定の証明書マネージャの個人タブから.p12をインポートします
- iOSは構成プロファイルとして.p12を取り込みます

CA秘密鍵`/etc/feedflow/mtls/ca.key`は配布せず、サーバ上だけに保管します。

## 6 TLSサーバ証明書

初回はnginxが443で起動できるよう、先に80だけでチャレンジを通します。`deploy/nginx/conf.d/feedflow.conf`の`server_name`を実ドメインへ書き換えてから取得します。

```bash
# server_nameを実ドメインへ置換します
sed -i 's/feedflow.example.com/your-real-domain.example/g' deploy/nginx/conf.d/feedflow.conf
# 証明書を取得します
sudo bash deploy/scripts/issue-tls-cert.sh your-real-domain.example you@example.com
```

## 7 起動

```bash
docker compose up -d --build
docker compose ps
# nginxの設定を検証します
docker compose exec nginx nginx -t
```

## 8 動作確認

```bash
# 証明書なしは403で拒否されます
curl -k -sS -o /dev/null -w "%{http_code}\n" https://your-real-domain.example/
# 期待値は403です

# クライアント証明書つきは到達します
curl --cert ./client-certs/okamyuji.crt --key ./client-certs/okamyuji.key \
  -sS -o /dev/null -w "%{http_code}\n" https://your-real-domain.example/healthz
# 期待値は200です
```

## 9 証明書の更新

Let's Encryptは90日で失効するため定期更新します。cronで月次更新しnginxをリロードします。

```bash
( sudo crontab -l 2>/dev/null; \
  echo "0 3 1 * * cd $HOME/feedflow-go-htmx && bash deploy/scripts/issue-tls-cert.sh your-real-domain.example you@example.com && docker compose exec -T nginx nginx -s reload" ) \
  | sudo crontab -
```

## 10 バックアップ

dataディレクトリはEBS上にあります。EBSスナップショットを定期取得します。アプリは全データをメモリ常駐しつつ`os.Rename`でアトミックにJSONへ書き込むため、スナップショット時点の整合は保たれます。
