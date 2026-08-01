# EC2本体と追加EBSとElastic IPを定義します。

# リポジトリの必要ファイルをtar.gzへまとめます。
# 配送対象はcmdとinternalとgo.modとgo.sumとDockerfileとcompose.ymlとdeployです。
# 除外対象は.gitやbinやdataやnode_modulesや.superpowersやdocsやe2eやpngです。
# archiveプロバイダはzipのみのためtar.gzはローカルのtarで生成します。
locals {
  tmp_dir     = abspath("${path.module}/.terraform-tmp")
  bundle_path = "${local.tmp_dir}/app_bundle.tar.gz"
  repo_root   = abspath("${path.module}/../..")

  # 配送するパスの一覧です。リポジトリルートからの相対で指定します。
  bundle_paths = "cmd internal go.mod go.sum Dockerfile compose.yml deploy"

  # tarから除外するパターンです。
  bundle_excludes = join(" ", [
    "--exclude=.git",
    "--exclude=bin",
    "--exclude=data",
    "--exclude=node_modules",
    "--exclude=.superpowers",
    "--exclude=docs",
    "--exclude=e2e",
    "--exclude=*.png",
    "--exclude=deploy/terraform",
  ])
}

# 配送対象のチェックサムでバンドル再生成のトリガーを作ります。
data "archive_file" "app_bundle_hash" {
  type        = "zip"
  output_path = "${path.module}/.terraform-tmp/app_bundle_hash.zip"
  source_dir  = local.repo_root

  excludes = [
    ".git",
    ".github",
    "bin",
    "data",
    "node_modules",
    ".superpowers",
    "docs",
    "e2e",
    "web",
    "coverage.out",
    "coverage.html",
    "deploy/terraform",
  ]
}

# tar.gzをローカルで生成します。中身が変わると再生成します。
resource "null_resource" "app_bundle" {
  triggers = {
    content_hash = data.archive_file.app_bundle_hash.output_sha256
  }

  provisioner "local-exec" {
    working_dir = local.repo_root
    command     = "mkdir -p ${local.tmp_dir} && tar ${local.bundle_excludes} -czf ${local.bundle_path} ${local.bundle_paths}"
  }
}

# EC2インスタンスを作成します。AMIはdata sourceで最新のAL2023 ARM64を使います。
resource "aws_instance" "feedflow" {
  ami                         = data.aws_ami.al2023_arm64.id
  instance_type               = var.instance_type
  subnet_id                   = data.aws_subnet.selected.id
  key_name                    = aws_key_pair.this.key_name
  vpc_security_group_ids      = [aws_security_group.feedflow.id]
  associate_public_ip_address = true

  # 起動時に追加EBSをマウントしfstabへ追記します。
  user_data = templatefile("${path.module}/templates/user_data.sh.tftpl", {})

  # ルートEBSはgp3にします。
  root_block_device {
    volume_type           = "gp3"
    volume_size           = var.root_volume_size
    delete_on_termination = true
    encrypted             = true
  }

  tags = {
    Name = "${var.project_name}-ec2"
  }

  # AMIの最新解決による意図しないインスタンス再作成を防ぎます (2026-08-01の障害の再発防止)。
  # AMIを更新したいときは terraform apply -replace=aws_instance.feedflow を明示的に実行します。
  lifecycle {
    ignore_changes = [ami]
  }
}

# データ永続化用の追加EBSをインスタンスと同じAZに作成します。
resource "aws_ebs_volume" "data" {
  availability_zone = data.aws_subnet.selected.availability_zone
  size              = var.data_volume_size
  type              = "gp3"
  encrypted         = true

  tags = {
    Name = "${var.project_name}-data"
  }
}

# 追加EBSをインスタンスへアタッチします。
resource "aws_volume_attachment" "data" {
  device_name = "/dev/sdf"
  volume_id   = aws_ebs_volume.data.id
  instance_id = aws_instance.feedflow.id
}

# Elastic IPを割り当てインスタンスへ関連付けます。
resource "aws_eip" "feedflow" {
  domain = "vpc"

  tags = {
    Name = "${var.project_name}-eip"
  }
}

resource "aws_eip_association" "feedflow" {
  instance_id   = aws_instance.feedflow.id
  allocation_id = aws_eip.feedflow.id
}
