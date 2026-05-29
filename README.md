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
