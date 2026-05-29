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

variable "client_cert_name" {
  description = "mTLS用に発行するクライアント証明書の名前を指定します。"
  type        = string
  default     = "owner"
}

variable "project_name" {
  description = "タグやキー名の接頭辞に使うプロジェクト名を指定します。"
  type        = string
  default     = "feedflow"
}
