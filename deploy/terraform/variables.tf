# 入力変数を定義します。既定値は最廉価かつ単一EC2構成に合わせます。

variable "region" {
  description = "デプロイ先のAWSリージョンを指定します。"
  type        = string
  default     = "ap-northeast-1"
}

variable "instance_type" {
  description = "EC2インスタンスタイプを指定します。ARM64のt4g系を前提とします。"
  type        = string
  default     = "t4g.small"
}

variable "data_volume_size" {
  description = "データ永続化用に追加するEBSの容量をGiBで指定します。"
  type        = number
  default     = 8
}

variable "root_volume_size" {
  description = "ルートEBSの容量をGiBで指定します。"
  type        = number
  default     = 20
}

variable "ssh_ingress_cidr" {
  description = "SSHを許可する送信元CIDRを指定します。空のままにすると実行環境のグローバルIPの/32を自動で使います。"
  type        = string
  default     = ""
}

variable "project_name" {
  description = "タグやキー名の接頭辞に使うプロジェクト名を指定します。"
  type        = string
  default     = "feedflow"
}

# Cloudflare関連の変数です。秘密値はsecrets.auto.tfvarsへ記入し、コードへは書きません。

variable "cloudflare_api_token" {
  description = "DNSとゾーン設定とAccessとOrigin CA証明書を操作するCloudflare APIトークンですDNS編集とゾーン設定編集とゾーン読み取りとSSL and Certificates編集とAccess AppsとPolicies編集の権限が要ります。"
  type        = string
  sensitive   = true
}

variable "cloudflare_account_id" {
  description = "Cloudflare AccessアプリをひもづけるアカウントIDです。"
  type        = string
}

variable "zone_name" {
  description = "Cloudflareで管理しているゾーン名です。"
  type        = string
  default     = "okamyuji.work"
}

variable "hostname" {
  description = "アプリを公開する完全修飾ホスト名です。AレコードとAccessとOrigin証明書に使います。"
  type        = string
  default     = "feedflow.okamyuji.work"
}

variable "access_owner_email" {
  description = "Cloudflare Accessで通過を許可する所有者のメールアドレスです。"
  type        = string
  default     = "owner@example.com"
}
