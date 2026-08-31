output "api_gateway_url" {
  value = module.automata.api_gateway_url
}

output "api_lambda_function_name" {
  value = module.automata.api_lambda_function_name
}

output "scheduler_lambda_function_name" {
  value = module.automata.scheduler_lambda_function_name
}

output "worker_lambda_function_name" {
  value = module.automata.worker_lambda_function_name
}

output "migrate_lambda_function_name" {
  value = module.automata.migrate_lambda_function_name
}

output "jobs_table_name" {
  value = module.automata.jobs_table_name
}

output "failure_bucket_name" {
  value = module.automata.failure_bucket_name
}
