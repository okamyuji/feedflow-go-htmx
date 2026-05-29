# feedflow-go-htmxの実装計画オーバービュー

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement each phase plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: Web上で動作する個人専用のRSS Readerを、Goの標準ライブラリ中心とHTMXとAlpine.jsで実装し、JSONファイル永続化と単一EC2デプロイで完成させます。

Architecture: クリーンアーキテクチャで各層をinternal/portのインターフェースで疎結合します。全データをメモリ常駐させアトミックJSONで永続化します。フィードはバックグラウンドで定期ポーリングし、HTMXで部分更新します。CloudflareプロキシとCloudflare Accessとアプリログインの二層で所有者だけに限定します(当初設計のnginx mTLSは廃止しました)。

Tech Stack: Go(標準ライブラリ)、golang.org/x/net/html、golang.org/x/crypto、HTMX、Alpine.js(CSPビルドの@alpinejs/csp)、Docker Compose、nginx、Playwright(TypeScript)。CSPのscript-src selfと両立させるため、unsafe-evalを要求する標準ビルドではなくCSPビルドを採用します。

---

## 0 この計画書群の使い方

この計画はフェーズ別の複数ファイルに分かれます。各フェーズは独立してビルドとテストが通る単位です。番号順に実装します。

| フェーズ | ファイル | 内容 | 主な成果物 |
| --- | --- | --- | --- |
| Phase0 | 01-foundation.md | 基盤と品質ゲート | go.mod、品質ゲート一式、最小サーバ、CI |
| Phase1 | 02-domain-port.md | ドメインとポート | エンティティ型、インターフェース定義 |
| Phase2 | 03-store.md | 永続化エンジン | メモリ常駐とアトミックJSONリポジトリ |
| Phase3 | 04-feed.md | フィード取得とパース | HTTP取得、RSS/Atom/RDFパース、本文抽出、自動検出 |
| Phase4 | 05-service.md | サービス層 | 購読管理、保持ポリシー、ミュート、OPML、整理 |
| Phase5 | 06-poller.md | ポーラー | 定期ポーリング、手動更新 |
| Phase6 | 07-auth.md | 認証とセキュリティ | scrypt、セッション、初回セットアップ、CSRF、レートリミット、ヘッダ |
| Phase7 | 08-handler-ui.md | ハンドラとUI | ルーティング、テンプレート、HTMX/Alpine、CSS |
| Phase8 | 09-deploy.md | デプロイ | terraformでCloudflareプロキシとAccessとOrigin CA、compose、Dockerfile、EC2手順(当初のmTLSとLet's Encryptは廃止) |
| Phase9 | 10-e2e.md | E2Eと堅牢化検証 | Playwright、verify-hardening.sh |

## 1 共通規約(全フェーズで厳守)

- テスト駆動で進めます。失敗するテストを先に書き、最小実装で通し、リファクタします
- 各タスクの最後でコミットします。コミットメッセージは規約に従います(feat、fix、refactor、docs、test、choreなど)
- 各フェーズの最後で`bash scripts/quality-gate.sh`を実行し、緑のまま次へ進みます
- 関数と型とパッケージのコメントは「識別子 説明」形式で書きます。識別子直後に助詞「は」を入れません。たとえば`// Feed フィードの購読単位を表します`と書きます。この形式で許される半角スペースは先頭の識別子直後の1個だけです
- ドキュメントとコメントの日本語は、絵文字とアスタリスク強調とコロン文末と体言止めを使わず、ですます調の単文で書きます。日本語と英単語と数字の境界に不要な半角スペースを入れません。これはGoコメントの本文にも同じく適用し、たとえば「セクション 4.2 に」ではなく「セクション4.2に」、「ETag です」ではなく「ETagです」、「外部 I/O を」ではなく「外部I/Oを」と書きます
- エラーは握り潰しません。defer Closeやrows.Closeやレスポンスボディのクローズは、internal/obsのログ付きヘルパ経由で記録します
- エラーは文脈付きでラップします。`fmt.Errorf("failed to load feeds: %w", err)`の形を用います
- 各層はインターフェースに依存し、依存はコンストラクタ注入で渡します
- SQLは使いません。動的SQL構築の論点はありません
- カバレッジは80パーセントを目標にします。厳密な合否基準ではなくCIではfailさせませんが、各フェーズで可能な限り維持します

## 2 ファイル構造マップ

各ファイルは単一の責務を持ちます。下記は最終形の目安です。フェーズ進行に応じて追加します。

```
feedflow-go-htmx/
├── cmd/feedflow/
│   ├── main.go                   エントリ、設定読込、依存組み立て、サーバとポーラー起動
│   └── main_test.go
├── internal/
│   ├── domain/                   エンティティと値オブジェクト(I/Oなし、純粋)
│   │   ├── feed.go               Feed
│   │   ├── category.go           Category
│   │   ├── item.go               Item、保持除外判定
│   │   ├── board.go              Board
│   │   ├── filter.go             MuteFilter、文字列一致判定
│   │   ├── settings.go           Settings
│   │   └── user.go               User
│   ├── port/                     インターフェース定義
│   │   ├── repository.go         Repository
│   │   ├── fetcher.go            Fetcher、FetchResult
│   │   ├── parser.go             FeedParser、ParsedFeed
│   │   ├── clock.go              Clock
│   │   ├── idgen.go              IDGen
│   │   └── service.go            サービスのインターフェース群
│   ├── sys/                      port.Clockとport.IDGenの本番用具象実装
│   │   ├── clock.go              SystemClock、実時刻を返すport.Clock実装
│   │   └── idgen.go              RandomIDGen、crypto/randによるport.IDGen実装
│   ├── store/                    永続化実装
│   │   ├── store.go              メモリ常駐の集約、ロードと保存
│   │   ├── atomic.go             temp+renameのアトミック書き込み
│   │   └── store_test.go
│   ├── feed/                     取得とパース
│   │   ├── fetcher.go            HTTP取得、ETag/Last-Modified、gzip、SSRF拒否
│   │   ├── parser.go             RSS2.0/Atom/RDF判別とパース
│   │   ├── discover.go           HTMLからのfeed link自動検出
│   │   └── extract.go            golang.org/x/net/htmlによる本文抽出
│   ├── service/                  業務ロジック
│   │   ├── subscription.go       購読追加と削除と一覧と整理
│   │   ├── item.go               既読、スター、あとで読む、タグ、ボード、メモ
│   │   ├── retention.go          保持ポリシー適用
│   │   ├── mute.go               ミュートフィルタ適用
│   │   ├── opml.go               OPML入出力
│   │   └── settings.go           設定の取得と更新
│   ├── poller/
│   │   └── poller.go             定期ポーリングと手動更新
│   ├── auth/
│   │   ├── password.go           scryptハッシュと検証
│   │   ├── session.go            メモリ保持のCookieセッション
│   │   ├── csrf.go               CSRFトークン
│   │   ├── ratelimit.go          標準ライブラリの簡易トークンバケット
│   │   └── setup.go              初回セットアップの可否判定
│   ├── handler/
│   │   ├── router.go             ルーティング登録
│   │   ├── middleware.go         認証、CSRF、セキュリティヘッダ、レートリミット
│   │   ├── render.go             embedテンプレートのParseFSとFuncMap
│   │   ├── auth_handler.go       ログインと初回セットアップ
│   │   ├── feed_handler.go       購読の追加削除一覧
│   │   ├── item_handler.go       記事一覧、本文オーバーレイ、既読、スター操作
│   │   ├── board_handler.go      ボード操作
│   │   ├── settings_handler.go   設定とOPML
│   │   ├── static.go             embed静的資産の配信、SHA256のETagと再検証
│   │   ├── templates/            base.htmlと_部分テンプレート、render.goが同パッケージ相対でembed
│   │   └── static/               htmx.min.js、alpine.min.js、styles.css、app.js、static.goが同パッケージ相対でembed
│   └── obs/
│       └── obs.go                CloseAndLog、WriteAndLog
├── data/                         実行時生成、gitignore対象
├── deploy/
│   ├── nginx/                    nginx.conf、Cloudflare向けリバースプロキシ設定
│   ├── terraform/                AWSとCloudflareをまとめて管理するterraform一式
│   └── README.md                 EC2とCloudflareとterraformの手順
├── e2e/playwright/               Playwright(TypeScript)
├── scripts/
│   ├── quality-gate.sh
│   ├── verify-hardening.sh
│   └── hooks/check_gofmt.sh
├── .github/workflows/ci.yml
├── compose.yml
├── Dockerfile
├── Makefile
├── .golangci.yml
├── .gitleaks.toml
├── .pre-commit-config.yaml
├── .gitignore
├── .env.example
├── README.md
└── go.mod
```

## 3 依存順序と並行可能性

- Phase0は最初に行います
- Phase1はPhase0の後に行います
- Phase2とPhase3はPhase1の後で着手でき、互いに独立です
- Phase4はPhase2とPhase3の後に行います
- Phase5はPhase4の後に行います
- Phase6はPhase1の後で着手でき、Phase2の永続化を使います
- Phase7はPhase4とPhase6の後に行います
- Phase8はPhase7の後に行います
- Phase9はPhase7の後に行い、Phase8の構成も検証します

## 4 各フェーズの完了条件(DOD)

- `bash scripts/quality-gate.sh`が緑であること
- そのフェーズで追加した機能のユニットテストが通ること
- 追加コードのコメントと日本語が共通規約に従うこと
- コミットが規約に沿って積まれていること
