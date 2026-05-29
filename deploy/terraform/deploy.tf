# アプリの配送と起動を行います。git remoteが無いためローカルからtar.gzを転送します。
# SSH到達のためElastic IPの関連付け完了後に実行します。

resource "null_resource" "deploy" {
  # バンドル内容やインスタンスやEIPが変わると再実行します。
  triggers = {
    bundle_hash = data.archive_file.app_bundle_hash.output_sha256
    instance_id = aws_instance.feedflow.id
    eip         = aws_eip.feedflow.public_ip
  }

  depends_on = [
    null_resource.app_bundle,
    aws_eip_association.feedflow,
    aws_volume_attachment.data,
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

  # 自己署名用のnginx confをEC2上だけに配置します。元リポジトリのconfは書き換えません。
  provisioner "file" {
    content     = templatefile("${path.module}/templates/feedflow.conf.tftpl", {})
    destination = "/home/ec2-user/feedflow.selfsigned.conf"
  }

  # 自己署名用のcompose上書きを配置します。
  provisioner "file" {
    content     = file("${path.module}/templates/compose.override.yml.tftpl")
    destination = "/home/ec2-user/compose.override.yml"
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

      # 追加EBSは起動時のuser_dataで/mnt/feedflow-dataへマウント済みです。念のため確認します。
      "mount | grep -q /mnt/feedflow-data || (echo 'data volume not mounted' >&2; exit 1)",

      # アプリを展開します。
      "rm -rf /home/ec2-user/feedflow && mkdir -p /home/ec2-user/feedflow",
      "tar -xzf /home/ec2-user/app_bundle.tar.gz -C /home/ec2-user/feedflow",

      # mTLSのCAを生成します。
      "sudo bash /home/ec2-user/feedflow/deploy/scripts/make-mtls-ca.sh /etc/feedflow/mtls",

      # クライアント証明書を生成します。出力はホームのclient-certsへ置きます。
      "cd /home/ec2-user/feedflow && bash deploy/scripts/make-client-cert.sh /etc/feedflow/mtls ${var.client_cert_name} /home/ec2-user/client-certs",
      "sudo chown -R ec2-user:ec2-user /home/ec2-user/client-certs",

      # 自己署名TLS証明書を生成します。CNとsubjectAltNameにElastic IPを設定し有効期間は365日にします。
      "sudo mkdir -p /etc/feedflow/tls",
      "sudo openssl req -x509 -nodes -newkey rsa:2048 -days 365 -keyout /etc/feedflow/tls/server.key -out /etc/feedflow/tls/server.crt -subj \"/CN=${aws_eip.feedflow.public_ip}\" -addext \"subjectAltName=IP:${aws_eip.feedflow.public_ip}\"",

      # 自己署名用のnginx confをfeedflow.confが参照する配置先へ置きます。
      "sudo mkdir -p /etc/feedflow/conf.d",
      "sudo cp /home/ec2-user/feedflow.selfsigned.conf /etc/feedflow/conf.d/feedflow.conf",

      # compose上書きをリポジトリ展開先へ配置します。
      "cp /home/ec2-user/compose.override.yml /home/ec2-user/feedflow/compose.override.yml",

      # .envを生成します。FEEDFLOW_BASE_URLはElastic IPのhttpsにしSESSION_KEYはランダム生成します。
      "cd /home/ec2-user/feedflow && printf 'FEEDFLOW_BASE_URL=https://%s\\nFEEDFLOW_SESSION_KEY=%s\\n' '${aws_eip.feedflow.public_ip}' \"$(openssl rand -base64 32)\" > .env",

      # 起動します。dockerグループ反映のためsudoで実行します。
      "cd /home/ec2-user/feedflow && sudo docker compose --env-file .env -f compose.yml -f compose.override.yml up -d --build",

      # 起動状態とnginx設定を確認します。
      "cd /home/ec2-user/feedflow && sudo docker compose -f compose.yml -f compose.override.yml ps",
      "cd /home/ec2-user/feedflow && sudo docker compose -f compose.yml -f compose.override.yml exec -T nginx nginx -t",
    ]
  }
}
