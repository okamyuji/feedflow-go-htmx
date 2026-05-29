# applyの結果として確認に使う値と運用手順を出力します。

output "elastic_ip" {
  description = "割り当てたElastic IPです。ブラウザやcurlの接続先になります。"
  value       = aws_eip.feedflow.public_ip
}

output "ssh_command" {
  description = "EC2へSSH接続するコマンドの例です。"
  value       = "ssh -i ${path.module}/feedflow_ssh_key.pem ec2-user@${aws_eip.feedflow.public_ip}"
}

output "fetch_client_cert_p12" {
  description = "ブラウザ取り込み用のクライアント証明書p12をローカルへ取得するscpの例です。"
  value       = "scp -i ${path.module}/feedflow_ssh_key.pem ec2-user@${aws_eip.feedflow.public_ip}:/home/ec2-user/client-certs/${var.client_cert_name}.p12 ./${var.client_cert_name}.p12"
}

output "fetch_client_cert_crt_key" {
  description = "curl検証用のクライアント証明書crtとkeyをローカルへ取得するscpの例です。"
  value       = "scp -i ${path.module}/feedflow_ssh_key.pem ec2-user@${aws_eip.feedflow.public_ip}:/home/ec2-user/client-certs/${var.client_cert_name}.crt ./${var.client_cert_name}.crt && scp -i ${path.module}/feedflow_ssh_key.pem ec2-user@${aws_eip.feedflow.public_ip}:/home/ec2-user/client-certs/${var.client_cert_name}.key ./${var.client_cert_name}.key"
}

output "healthcheck_with_cert" {
  description = "クライアント証明書つきでhealthzへアクセスし200を期待するcurlの例です。自己署名のため-kで検証を省きます。"
  value       = "curl -k --cert ./${var.client_cert_name}.crt --key ./${var.client_cert_name}.key -sS -o /dev/null -w '%%{http_code}\\n' https://${aws_eip.feedflow.public_ip}/healthz"
}

output "healthcheck_without_cert" {
  description = "クライアント証明書なしでアクセスし403を期待するcurlの例です。"
  value       = "curl -k -sS -o /dev/null -w '%%{http_code}\\n' https://${aws_eip.feedflow.public_ip}/healthz"
}
