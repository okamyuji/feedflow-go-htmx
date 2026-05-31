# Terraform本体とプロバイダのバージョン制約をまとめます。
# AWSとCloudflareの両方をterraformで管理します。

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "~> 4.40"
    }
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
    local = {
      source  = "hashicorp/local"
      version = "~> 2.5"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.2"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
    http = {
      source  = "hashicorp/http"
      version = "~> 3.4"
    }
  }
}

provider "aws" {
  region  = var.region
  profile = var.aws_profile != "" ? var.aws_profile : null
}

# CloudflareプロバイダですDNSとゾーン設定とAccessとOrigin CA証明書の発行をすべてAPIトークンで
# 操作します。プロバイダv3.32.0以降はOrigin CA証明書もトークンで発行できるためOrigin CA Keyは
# 使いません。トークンにはSSL and Certificatesの編集権限を含めます。
provider "cloudflare" {
  api_token = var.cloudflare_api_token
}
