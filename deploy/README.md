# feedflow デプロイ手順

AWS の単一 EC2(ARM の t4g 系)と Cloudflare を terraform でまとめて管理してデプロイします。ALB と NLB は使わず、nginx コンテナと Go アプリコンテナを同居させます。本人限定は Cloudflare Access が担い、当初設計の nginx mTLS と Let's Encrypt と certbot は廃止しました。

## 構成図

```
[ブラウザ]
   | HTTPS
   v
[Cloudflare エッジ] -- Access(所有者メールのみ許可) / プロキシでIP秘匿 / Full strict
   | HTTPS(Cloudflare IP範囲のみ到達可)
   v
[EC2 t4g] -- compose --+-- nginx(443終端 / Origin CA証明書 / リバースプロキシ)
                       |        | 内部ネットワーク8080
                       +-- app(feedflow単一バイナリ / embed同梱)
                                | /data
                       [EBSマウント /mnt/feedflow-data]
```

## セキュリティの要点

- Cloudflare Access がホスト名全体を保護し、所有者メールだけを通過させます。未認証はエッジで止まります
- セキュリティグループは 443 と 80 を Cloudflare の IP 範囲だけに限定し、SSH は運用者 IP のみにします。これで Access を迂回したオリジン直アクセスを塞ぎます
- オリジンは Cloudflare Origin CA 証明書を配置し SSL モードを Full(strict)にします。Origin CA Key は非推奨のため使わず、terraform の管理する API トークンで発行します
- アプリ自身が scrypt と Cookie セッションでもう一段守ります

## 前提

- AWS は IAM Identity Center の SSO で認証します。長期アクセスキーは使いません。`aws sso login --profile <profile>` で都度ログインし、以降のコマンドに `AWS_PROFILE=<profile>` を付けます
- Cloudflare で対象ゾーン(例 okamyuji.work)を管理済みで、Zero Trust(Access)を有効化済みであること
- terraform 1.6 以降、AWS CLI v2、ローカルに git があること

## 1 Cloudflare の秘密値を用意する

ユーザー API トークンを作成します。マイプロフィールの API トークンからカスタムトークンを発行し、次の権限を付けます。権限グループ名は日本語ダッシュボードでも英語表記です。

- ゾーン DNS 編集(対象ゾーン)
- ゾーン ゾーン設定 編集(対象ゾーン)
- ゾーン ゾーン 読み取り(対象ゾーン)
- ゾーン SSL and Certificates 編集(対象ゾーン)
- アカウント Access: Apps 編集
- アカウント Access: Policies 編集

アカウント ID も控えます。これらを gitignore 済みのファイルへ記入します。

```bash
cd deploy/terraform
cp secrets.auto.tfvars.example secrets.auto.tfvars
# エディタで cloudflare_api_token と cloudflare_account_id を記入する
```

## 2 変数の既定値

`variables.tf` の既定は次のとおりです。必要なら secrets.auto.tfvars や別の tfvars で上書きします。

- `region` ap-northeast-1
- `instance_type` t4g.small
- `zone_name` okamyuji.work
- `hostname` feedflow.okamyuji.work
- `access_owner_email` okamyuji@gmail.com
- `ssh_ingress_cidr` 空のときは実行環境のグローバル IP の /32 を自動で使う

## 3 init と plan と apply

```bash
cd deploy/terraform
aws sso login --profile <profile>

AWS_PROFILE=<profile> terraform init
AWS_PROFILE=<profile> terraform plan
AWS_PROFILE=<profile> terraform apply
```

apply で作成されるものは、Elastic IP 付きの EC2 と追加 EBS、Cloudflare の A レコード(プロキシ ON)、SSL モード Full(strict)、Origin CA 証明書、Zero Trust Access のアプリと所有者許可ポリシー、Cloudflare IP に限定したセキュリティグループです。アプリは SSH プロビジョナで配送し EC2 上で docker compose ビルドします。Amazon Linux 2023 の既定 buildx は古く compose build が 0.17.0 以上を要求するため、最新 buildx を起動時に手動導入します。

## 4 出力と動作確認

```bash
AWS_PROFILE=<profile> terraform output
```

主な出力は次のとおりです。

- `app_url` 公開 URL(例 https://feedflow.okamyuji.work)
- `elastic_ip` 割り当てた EIP
- `dns_record` 作成した A レコード(proxied)
- `access_application` Access が保護するドメイン
- `ssh_command` EC2 への SSH 例

ブラウザで `app_url` を開くと、まず Cloudflare Access の認証(所有者メールへのアクセスコード)を求められます。通過後にアプリのログイン画面が出ます。初回はオーナー未登録のためセットアップ画面になり、ユーザー名とパスワードを登録します。

## 5 更新の反映

アプリのソースを変更したら、同じ apply を再実行します。バンドルのハッシュが変わると EC2 でイメージを再ビルドしてコンテナを再起動します。静的資産は URL にコンテンツハッシュを付けているため、ブラウザと Cloudflare エッジは確実に最新を取得します。

```bash
AWS_PROFILE=<profile> terraform apply
```

## 6 バックアップ

data ディレクトリは EBS 上の `/mnt/feedflow-data` にあります。EBS スナップショットを定期取得します。アプリは全データをメモリ常駐しつつ `os.Rename` でアトミックに JSON へ書き込むため、スナップショット時点の整合は保たれます。

## 7 取り消し

```bash
AWS_PROFILE=<profile> terraform destroy
```

AWS と Cloudflare の双方の作成物をまとめて削除します。
