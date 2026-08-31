locals {
  common_tags = {
    service     = "automata"
    environment = var.environment
    managed_by  = "terraform"
  }

  enable_hosted = coalesce(var.enable_hosted, var.environment != "local")

  lambda_aws_architecture = lower(trimspace(var.lambda_architecture)) == "arm64" ? "arm64" : "x86_64"
  lambda_go_arch = (
    lower(trimspace(var.go_arch)) == "arm64" ? "arm64" :
    contains(["amd64", "x86_64"], lower(trimspace(var.go_arch))) ? "amd64" :
    (local.lambda_aws_architecture == "arm64" ? "arm64" : "amd64")
  )

  dsql_endpoint = local.enable_hosted ? format("%s.dsql.%s.on.aws", aws_dsql_cluster.hosted[0].identifier, var.aws_region) : ""

  effective_database_engine = trimspace(var.database_engine) != "" ? trimspace(var.database_engine) : (local.enable_hosted ? "dsql" : "postgres")
  runtime_database_url      = local.enable_hosted ? "postgres://${var.dsql_runtime_role}@${local.dsql_endpoint}/${var.dsql_database_name}?sslmode=require" : ""
  migrate_database_url      = local.enable_hosted ? "postgres://${var.dsql_admin_role}@${local.dsql_endpoint}/${var.dsql_database_name}?sslmode=require" : local.runtime_database_url
  effective_database_url    = trimspace(var.database_url) != "" ? trimspace(var.database_url) : local.runtime_database_url

  effective_app_public_url = trimspace(var.app_public_url) != "" ? trimspace(var.app_public_url) : (
    local.enable_hosted && trimspace(var.api_domain_name) != "" ? "https://${trimspace(var.api_domain_name)}" : ""
  )
  effective_dashboard_base_url = trimspace(var.dashboard_base_url) != "" ? trimspace(var.dashboard_base_url) : ""


  effective_cors_origins_csv = trimspace(var.cors_origins_csv) != "" ? trimspace(var.cors_origins_csv) : join(",", var.cors_origins)
  foundation_model_id = (
    startswith(var.bedrock_model_id, "eu.") ? trimprefix(var.bedrock_model_id, "eu.") :
    startswith(var.bedrock_model_id, "us.") ? trimprefix(var.bedrock_model_id, "us.") :
    startswith(var.bedrock_model_id, "global.") ? trimprefix(var.bedrock_model_id, "global.") :
    var.bedrock_model_id
  )

  lambda_env = merge(
    {
      APP_PUBLIC_URL                 = local.effective_app_public_url
      LISTEN_ADDR                    = ":8080"
      DATABASE_ENGINE                = local.effective_database_engine
      DATABASE_URL                   = local.effective_database_url
      DASHBOARD_BASE_URL             = local.effective_dashboard_base_url
      CORS_ORIGINS                   = local.effective_cors_origins_csv
      AUTH_DEFAULT_USER_ID           = var.auth_default_user_id
      MS_CLIENT_ID                   = var.ms_client_id
      MS_REDIRECT_URI                = var.ms_redirect_uri
      MS_AUTH_REDIRECT_URI           = var.ms_auth_redirect_uri
      GOOGLE_CLIENT_ID               = var.google_client_id
      GOOGLE_REDIRECT_URI            = var.google_redirect_uri
      SLACK_MODE                     = var.slack_mode
      SLACK_CLIENT_ID                = var.slack_client_id
      SLACK_REDIRECT_URI             = var.slack_redirect_uri
      JOBS_INLINE                    = tostring(var.jobs_inline)
      JOBS_TABLE                     = aws_dynamodb_table.jobs.name
      JOB_TERMINAL_RETENTION_DAYS    = tostring(var.job_terminal_retention_days)
      JOB_LEASE_SECONDS              = tostring(var.job_lease_seconds)
      JOB_PENDING_WAKE_AFTER_SECONDS = tostring(var.job_pending_wake_after_seconds)
      # AWS_REGION is reserved by Lambda and injected automatically; do not set it.
      AWS_ENDPOINT                   = var.aws_endpoint
      BEDROCK_MODEL_ID               = var.bedrock_model_id
      BEDROCK_RUNTIME_ENDPOINT       = var.bedrock_runtime_endpoint
      DSQL_CLUSTER_ENDPOINT          = local.dsql_endpoint
      DSQL_REGION                    = local.enable_hosted ? var.aws_region : ""
      DSQL_DATABASE_ROLE             = local.enable_hosted ? var.dsql_runtime_role : ""
    },
    local.local_secret_env,
    local.hosted_secret_env,
  )

  migrate_lambda_env = merge(local.lambda_env, {
    DATABASE_URL       = local.enable_hosted ? local.migrate_database_url : local.effective_database_url
    DSQL_DATABASE_ROLE = local.enable_hosted ? var.dsql_admin_role : ""
  })
}
