# 外部から解決する値をdata sourceでまとめます。

# 最新のAmazon Linux 2023のARM64 AMIを解決します。
data "aws_ami" "al2023_arm64" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-2023.*-arm64"]
  }

  filter {
    name   = "architecture"
    values = ["arm64"]
  }

  filter {
    name   = "root-device-type"
    values = ["ebs"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# デフォルトVPCを参照します。新規VPCは作らず最廉価に寄せます。
data "aws_vpc" "default" {
  default = true
}

# デフォルトVPC内のサブネットを列挙します。
data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# 列挙したサブネットの先頭の詳細を取得しアベイラビリティゾーンを得ます。
# 追加EBSはインスタンスと同じAZへ作る必要があるため使います。
data "aws_subnet" "selected" {
  id = tolist(data.aws_subnets.default.ids)[0]
}

# 実行環境のグローバルIPを取得しSSH許可CIDRの既定値に使います。
data "http" "myip" {
  url = "https://checkip.amazonaws.com"
}

locals {
  # 末尾の改行を除いたグローバルIPです。
  detected_ip = chomp(data.http.myip.response_body)

  # 変数で上書きされていればそれを使い、なければ検出したIPの/32を使います。
  ssh_cidr = var.ssh_ingress_cidr != "" ? var.ssh_ingress_cidr : "${local.detected_ip}/32"
}
