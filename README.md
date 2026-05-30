# feedflow-go-htmx

Web 上で動作する個人専用の RSS Reader です。Go の標準ライブラリ中心と HTMX と Alpine.js で実装し、データは SQL を使わず JSON ファイルで永続化します。所有者ひとりだけが使う前提で、本番は Cloudflare Access による本人限定とアプリ自身のログインの二層で守ります。

## 主な機能

- フィード URL の登録とサイト URL からのフィード自動検出による購読追加
- OPML のインポートとエクスポート。インポートはバックグラウンドで進み、件数が多くてもリクエストはすぐ返ります
- すべてと各フィードの一覧は未読を中心に表示します。単一フィード表示時は、うっかり既読にした記事の再読のため直近の既読5件を先頭にまとめて再表示します
- 未読のあるフィードはフィード一覧の先頭に更新日時の新しい順でまとめて表示します。未読がなければ購読順のまま表示します
- 一覧をスクロールすると上端より上へ流れた未読記事を自動で既読にします。設定で ON と OFF を切り替えられます
- 一括既読は表示中の範囲とすべてのフィードを分離します。特定フィード表示中はこのフィードを既読、すべて表示中は表示中をすべて既読になり、すべてのフィードを既読は確認を挟みます
- サイドバーのフィード一覧をテキスト欄で部分一致絞り込みできます。入力のたびに即時で絞り込み、空にすると全表示へ戻ります
- 左ペインは選択中のノードを強調し、右ペイン左上に選択中の名称を表示します
- ブックマークによる保存。階層なしの名称付きで、保存時に新規作成か既存の名称への追加を選べます。左メニューのブックマークから全件や名称ごとに絞り込めます。元記事が消えた場合はエラーを出さず黙って非表示にします
- あとで読む、ミュートフィルタ、記事本文のオーバーレイ表示、元記事の本文抽出
- ライトとダークのテーマ切替。スマートフォン(iPhone/Android)向けにツリーをオフキャンバス・ドロワー化したレスポンシブ対応
- キーボードショートカット(j と k で移動、m で既読、b でブックマーク)

## 必要なもの

- Go 1.25 以降
- golangci-lint v2.12.2、staticcheck、govulncheck、gitleaks、pre-commit
- Node.js 24(E2E を実行する場合)

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

## 環境変数

| 変数 | 用途 | 既定 |
| --- | --- | --- |
| FEEDFLOW_ADDR | サーバの待受アドレス | :8080 |
| FEEDFLOW_DATA_DIR | JSON 永続化のデータディレクトリ | ./data |
| FEEDFLOW_BASE_URL | 公開時のベース URL。Cookie の Secure 判定や共有リンク生成に使う | (必須) |
| FEEDFLOW_SESSION_KEY | セッション署名鍵。openssl rand -base64 32 などで生成する | (必須) |
| FEEDFLOW_LOGIN_BURST | ログイン試行レート制限の同時許容回数。短時間に多数ログインする E2E で大きくする | 5 |
| FEEDFLOW_ALLOW_PRIVATE_FETCH | 値を入れるとプライベートやループバック宛のフィード取得を許可する。SSRF 対策を無効化するため本番では使わず E2E 専用 | 未設定(保護 ON) |

## ローカル起動

```bash
FEEDFLOW_ADDR=":8907" \
FEEDFLOW_DATA_DIR=/tmp/feedflow-local \
FEEDFLOW_BASE_URL=http://127.0.0.1:8907 \
FEEDFLOW_SESSION_KEY="$(openssl rand -base64 32)" \
  go run ./cmd/feedflow
```

初回は http://127.0.0.1:8907/ を開くとセットアップ画面が出るので、ユーザー名とパスワードを登録します。以後はそのログインで利用します。

## セキュリティモデル

本番のアプリログイン画面は全世界には公開しません。多層で囲います。

- Cloudflare Access がホスト名全体を保護し、所有者メールだけを通過させます。未認証は Cloudflare のエッジで止まります
- オリジンのセキュリティグループは 443 と 80 を Cloudflare の IP 範囲だけに限定し、SSH は運用者 IP のみにします。これにより Access を迂回したオリジン直アクセスを塞ぎます
- アプリ自身が scrypt と Cookie セッションでもう一段守ります

詳しくは設計書のセクション 9 を参照します。

## デプロイ

AWS の単一 EC2 と Cloudflare を terraform でまとめて管理します。手順は `deploy/README.md` を参照します。

## テスト

```bash
make test                              # Goの単体と統合テスト
cd e2e/playwright && npx playwright test   # PlaywrightのE2E
```

## 品質ゲート

`scripts/quality-gate.sh` を pre-commit と CI から同一コマンドで実行します。gofmt、go vet、staticcheck、golangci-lint、govulncheck、go test、gitleaks を通します。

## ドキュメント

- 設計書 `docs/superpowers/specs/2026-05-29-feedflow-design.md`
- 実装計画 `docs/superpowers/plans/`
- デプロイ手順 `deploy/README.md`
