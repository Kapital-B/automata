output "api_gateway_id" {
  value = aws_api_gateway_rest_api.api.id
}

output "api_gateway_url" {
  value = local.enable_hosted ? aws_api_gateway_stage.api.invoke_url : "http://localhost:4566/restapis/${aws_api_gateway_rest_api.api.id}/${var.environment}/_user_request_"
}

output "api_custom_domain_name" {
  value = local.enable_hosted ? aws_api_gateway_domain_name.api[0].domain_name : null
}

output "api_lambda_function_name" {
  value = aws_lambda_function.api.function_name
}

output "scheduler_lambda_function_name" {
  value = aws_lambda_function.scheduler.function_name
}

output "worker_lambda_function_name" {
  value = aws_lambda_function.worker.function_name
}

output "migrate_lambda_function_name" {
  value = aws_lambda_function.migrate.function_name
}

output "jobs_table_name" {
  value = aws_dynamodb_table.jobs.name
}

output "failure_bucket_name" {
  value = aws_s3_bucket.stream_failures.bucket
}

output "dsql_cluster_arn" {
  value = local.enable_hosted ? aws_dsql_cluster.hosted[0].arn : null
}

output "dsql_cluster_endpoint" {
  value = local.dsql_endpoint != "" ? local.dsql_endpoint : null
}

output "hosted_zone_name_servers" {
  description = "Nameservers for parent-zone NS delegation of the API hosted zone."
  value       = local.enable_hosted ? aws_route53_zone.public[0].name_servers : []
}

output "hosted_zone_name" {
  description = "API Route53 hosted zone name."
  value       = local.enable_hosted ? aws_route53_zone.public[0].name : null
}

output "secret_names" {
  description = "Secrets Manager secret names created for hosted envs. Populate values manually after apply."
  value = local.enable_hosted ? {
    encryption_key       = aws_secretsmanager_secret.encryption_key[0].name
    jwt_secret           = aws_secretsmanager_secret.jwt_secret[0].name
    job_cursor_secret    = aws_secretsmanager_secret.job_cursor_secret[0].name
    ms_client_secret     = aws_secretsmanager_secret.ms_client_secret[0].name
    google_client_secret = aws_secretsmanager_secret.google_client_secret[0].name
    slack_client_secret  = aws_secretsmanager_secret.slack_client_secret[0].name
  } : {}
}
