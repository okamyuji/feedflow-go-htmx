# セキュリティグループを定義します。443と80は全世界へ開きSSHは自分のIPだけに限定します。

resource "aws_security_group" "feedflow" {
  name        = "${var.project_name}-sg"
  description = "feedflow single EC2 ingress rules"
  vpc_id      = data.aws_vpc.default.id

  # 443はmTLSで保護するため全世界へ開きます。
  ingress {
    description = "HTTPS with mTLS protection"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # 80は443へのリダイレクト用に全世界へ開きます。
  ingress {
    description = "HTTP redirect to HTTPS"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
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
