# SSH鍵を生成しローカルへ秘密鍵を保存しAWSへ公開鍵を登録します。

# RSA4096のSSH鍵を生成します。
resource "tls_private_key" "ssh" {
  algorithm = "RSA"
  rsa_bits  = 4096
}

# 秘密鍵をローカルのpemファイルへ保存します。パーミッションは0600にします。
resource "local_file" "ssh_private_key" {
  content         = tls_private_key.ssh.private_key_pem
  filename        = "${path.module}/feedflow_ssh_key.pem"
  file_permission = "0600"
}

# 公開鍵をAWSのキーペアとして登録します。
resource "aws_key_pair" "this" {
  key_name   = "${var.project_name}-ssh-key"
  public_key = tls_private_key.ssh.public_key_openssh

  tags = {
    Name = "${var.project_name}-ssh-key"
  }
}
