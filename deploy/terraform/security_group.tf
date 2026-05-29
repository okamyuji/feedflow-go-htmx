# セキュリティグループを定義します。443と80はCloudflareのエッジIPだけに限定し、SSHは自分のIPだけに限定します。
# オリジンへの直接接続を遮断し、必ずCloudflareのプロキシとAccessを経由させます。

resource "aws_security_group" "feedflow" {
  name        = "${var.project_name}-sg"
  description = "feedflow single EC2 ingress rules"
  vpc_id      = data.aws_vpc.default.id

  # 443はCloudflareのエッジIPからのみ受けます。直接アクセスは遮断します。
  ingress {
    description = "HTTPS from Cloudflare edge only"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = data.cloudflare_ip_ranges.cloudflare.ipv4_cidr_blocks
  }

  # 80もCloudflareのエッジIPからのみ受けます。443へのリダイレクト用です。
  ingress {
    description = "HTTP from Cloudflare edge only"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = data.cloudflare_ip_ranges.cloudflare.ipv4_cidr_blocks
  }

  # SSHは実行環境のグローバルIPの/32だけに限定します。
  ingress {
    description = "SSH from operator IP only"
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = [local.ssh_cidr]
  }

  # 送信は全許可します。
  egress {
    description = "Allow all outbound"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.project_name}-sg"
  }
}
