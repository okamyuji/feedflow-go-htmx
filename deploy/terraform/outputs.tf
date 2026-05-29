# applyの結果として確認に使う値と運用手順を出力します。

output "elastic_ip" {
  description = "割り当てたElastic IPです。CloudflareのAレコードが指す先になります。"
  value       = aws_eip.feedflow.public_ip
}

output "app_url" {
  description = "公開URLです。Cloudflare AccessとアプリログインのあとにRSSリーダーへ入れます。"
  value       = "https://${var.hostname}"
}

output "ssh_command" {
  description = "EC2へSSH接続するコマンドの例です。"
  value       = "ssh -i ${path.module}/feedflow_ssh_key.pem ec2-user@${aws_eip.feedflow.public_ip}"
}

output "dns_record" {
  description = "作成したCloudflareのAレコードです。プロキシONでオリジンIPを秘匿します。"
  value       = "${cloudflare_record.app.name} A ${aws_eip.feedflow.public_ip} (proxied)"
}

output "access_application" {
  description = "本人限定を担うCloudflare Accessアプリの保護ドメインです。"
  value       = cloudflare_zero_trust_access_application.app.domain
}

output "healthcheck" {
  description = "公開URLのhealthzへアクセスし200を期待するcurlの例です。Access認証後のセッションが必要です。"
  value       = "curl -sS -o /dev/null -w '%%{http_code}\\n' https://${var.hostname}/healthz"
}
