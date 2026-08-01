# アプリの配送と起動を行います。git remoteが無いためローカルからtar.gzを転送します。
# SSH到達のためElastic IPの関連付け完了後に実行します。
# TLSはCloudflare Origin CA証明書を配置し、mTLSは廃止します。
#
# イメージはローカルでarm64ビルドして転送し、EC2側は docker load と起動だけを行います。
# t4g.micro (1GB RAM) 上でのGoビルドはメモリを食い尽くして同居アプリを巻き込むため
# 行いません (2026-08-01のboki3同居デプロイ時の障害の再発防止)。

locals {
  # ローカルでビルドしたイメージの書き出し先です。
  image_path = "${local.tmp_dir}/feedflow-image.tar.gz"
}

# arm64イメージをローカルでビルドしてtar.gzへ書き出します。バンドル内容が変わると再生成します。
resource "null_resource" "image" {
  triggers = {
    bundle_hash = data.archive_file.app_bundle_hash.output_sha256
  }

  provisioner "local-exec" {
    working_dir = local.repo_root
    command     = "mkdir -p ${local.tmp_dir} && docker buildx build --platform linux/arm64 --load -t feedflow:dev . && docker save feedflow:dev | gzip > ${local.image_path}"
  }
}

locals {
  # nginxのset_real_ip_fromディレクティブをCloudflareの全IP範囲から組み立てます。
  cloudflare_cidrs = concat(
    data.cloudflare_ip_ranges.cloudflare.ipv4_cidr_blocks,
    data.cloudflare_ip_ranges.cloudflare.ipv6_cidr_blocks,
  )
  real_ip_from_block = join("\n", [for c in local.cloudflare_cidrs : "set_real_ip_from ${c};"])
}

resource "null_resource" "deploy" {
  # バンドル内容やインスタンスやEIPや証明書が変わると再実行します。
  triggers = {
    bundle_hash = data.archive_file.app_bundle_hash.output_sha256
    instance_id = aws_instance.feedflow.id
    eip         = aws_eip.feedflow.public_ip
    cert_id     = cloudflare_origin_ca_certificate.origin.id
    hostname    = var.hostname
  }

  depends_on = [
    null_resource.app_bundle,
    null_resource.image,
    aws_eip_association.feedflow,
    aws_volume_attachment.data,
    cloudflare_origin_ca_certificate.origin,
  ]

  connection {
    type        = "ssh"
    host        = aws_eip.feedflow.public_ip
    user        = "ec2-user"
    private_key = tls_private_key.ssh.private_key_pem
    timeout     = "5m"
  }

  # アプリのtar.gzをホームへ転送します。
  provisioner "file" {
    source      = local.bundle_path
    destination = "/home/ec2-user/app_bundle.tar.gz"
  }

  # ローカルでビルド済みのイメージを転送します。EC2上ではビルドしません。
  provisioner "file" {
    source      = local.image_path
    destination = "/home/ec2-user/feedflow-image.tar.gz"
  }

  # Cloudflare構成のnginx confをEC2上だけに生成し配置します。元リポジトリのconfは書き換えません。
  provisioner "file" {
    content = templatefile("${path.module}/templates/feedflow.cloudflare.conf.tftpl", {
      hostname           = var.hostname
      real_ip_from_block = local.real_ip_from_block
    })
    destination = "/home/ec2-user/feedflow.cloudflare.conf"
  }

  # compose上書きを配置します。
  provisioner "file" {
    content     = file("${path.module}/templates/compose.override.yml.tftpl")
    destination = "/home/ec2-user/compose.override.yml"
  }

  # Origin CA証明書とその秘密鍵を配置します。Full(strict)でCloudflareが検証する証明書です。
  provisioner "file" {
    content     = cloudflare_origin_ca_certificate.origin.certificate
    destination = "/home/ec2-user/server.crt"
  }

  provisioner "file" {
    content     = tls_private_key.origin.private_key_pem
    destination = "/home/ec2-user/server.key"
  }

  # 展開と各種セットアップと起動を行います。
  provisioner "remote-exec" {
    inline = [
      "set -eux",

      # Dockerを導入し有効化しec2-userをdockerグループへ追加します。
      "sudo dnf install -y docker tar unzip",
      "sudo systemctl enable --now docker",
      "sudo usermod -aG docker ec2-user",

      # composeプラグインを導入します。deploy/README.mdの手順に従います。
      "sudo mkdir -p /usr/libexec/docker/cli-plugins",
      "ARCH=\"$(uname -m)\"",
      "sudo curl -fsSL \"https://github.com/docker/compose/releases/latest/download/docker-compose-linux-$${ARCH}\" -o /usr/libexec/docker/cli-plugins/docker-compose",
      "sudo chmod +x /usr/libexec/docker/cli-plugins/docker-compose",
      "sudo docker compose version",

      # buildxプラグインを導入します。compose buildが新しいbuildxを要求するため最新版を入れます。
      # buildxの配布ファイル名はarm64やamd64の表記なのでuname -mの結果を変換します。
      "case \"$${ARCH}\" in aarch64) BX_ARCH=arm64;; x86_64) BX_ARCH=amd64;; *) BX_ARCH=\"$${ARCH}\";; esac",
      "BUILDX_VER=\"$(curl -fsSL https://api.github.com/repos/docker/buildx/releases/latest | grep -m1 tag_name | cut -d '\"' -f4)\"",
      "sudo curl -fsSL \"https://github.com/docker/buildx/releases/download/$${BUILDX_VER}/buildx-$${BUILDX_VER}.linux-$${BX_ARCH}\" -o /usr/libexec/docker/cli-plugins/docker-buildx",
      "sudo chmod +x /usr/libexec/docker/cli-plugins/docker-buildx",
      "sudo docker buildx version",

      # 追加EBSは起動時のuser_dataで/mnt/feedflow-dataへマウント済みです。念のため確認します。
      "mount | grep -q /mnt/feedflow-data || (echo 'data volume not mounted' >&2; exit 1)",

      # アプリを展開します。
      "rm -rf /home/ec2-user/feedflow && mkdir -p /home/ec2-user/feedflow",
      "tar -xzf /home/ec2-user/app_bundle.tar.gz -C /home/ec2-user/feedflow",

      # Origin CA証明書と鍵を配置します。鍵は所有者のみ読めるようにします。
      "sudo mkdir -p /etc/feedflow/tls",
      "sudo cp /home/ec2-user/server.crt /etc/feedflow/tls/server.crt",
      "sudo cp /home/ec2-user/server.key /etc/feedflow/tls/server.key",
      "sudo chmod 600 /etc/feedflow/tls/server.key",
      "rm -f /home/ec2-user/server.crt /home/ec2-user/server.key",

      # Cloudflare構成のnginx confをfeedflow.confが参照する配置先へ置きます。
      "sudo mkdir -p /etc/feedflow/conf.d",
      "sudo cp /home/ec2-user/feedflow.cloudflare.conf /etc/feedflow/conf.d/feedflow.conf",

      # compose上書きをリポジトリ展開先へ配置します。
      "cp /home/ec2-user/compose.override.yml /home/ec2-user/feedflow/compose.override.yml",

      # .envを生成します。FEEDFLOW_BASE_URLは公開ホスト名のhttpsにしSESSION_KEYはランダム生成します。
      "cd /home/ec2-user/feedflow && printf 'FEEDFLOW_BASE_URL=https://%s\\nFEEDFLOW_SESSION_KEY=%s\\n' '${var.hostname}' \"$(openssl rand -base64 32)\" > .env",

      # ビルド済みイメージを取り込みます。EC2上ではビルドを行いません。
      "sudo docker load -i /home/ec2-user/feedflow-image.tar.gz",
      "rm -f /home/ec2-user/feedflow-image.tar.gz",

      # 起動します。dockerグループ反映のためsudoで実行します。イメージは転送済みのものを使います。
      "cd /home/ec2-user/feedflow && sudo docker compose --env-file .env -f compose.yml -f compose.override.yml up -d --no-build",

      # 起動状態とnginx設定を確認します。
      "cd /home/ec2-user/feedflow && sudo docker compose -f compose.yml -f compose.override.yml ps",
      "cd /home/ec2-user/feedflow && sudo docker compose -f compose.yml -f compose.override.yml exec -T nginx nginx -t",
    ]
  }
}
