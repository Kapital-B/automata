variable "environment" {
  description = "Deployment environment name."
  type        = string
}

variable "aws_region" {
  description = "Primary AWS region for the service deployment."
  type        = string
}

variable "enable_hosted" {
  description = "Whether to create hosted-only AWS resources such as DSQL and public DNS."
  type        = bool
  default     = null
}

variable "aws_endpoint" {
  description = "Optional local AWS endpoint override for Floci."
  type        = string
  default     = ""
}

variable "database_engine" {
  description = "Explicit database engine override. Defaults to postgres for local and dsql for hosted."
  type        = string
  default     = ""
}

variable "database_url" {
  description = "Explicit database connection URL override."
  type        = string
  default     = ""
}

variable "go_arch" {
  description = "GOARCH value used when building Lambda bootstrap binaries."
  type        = string
  default     = ""
}

variable "lambda_architecture" {
  description = "Lambda architecture name, such as x86_64 or arm64."
  type        = string
}

variable "app_public_url" {
  description = "Public base URL for the API."
  type        = string
  default     = ""
}

variable "dashboard_base_url" {
  description = "Public base URL for the web dashboard."
  type        = string
  default     = ""
}

variable "hosted_zone_name" {
  description = "Route53 public zone for the API hostname (e.g. api.automata-dev.kapital-b.com). SPA apex is owned by the frontend account."
  type        = string
  default     = ""
}

variable "api_domain_name" {
  description = "API custom domain name for hosted environments."
  type        = string
  default     = ""
}

variable "cors_origins" {
  description = "List of CORS origins to join into the runtime CORS_ORIGINS value."
  type        = list(string)
  default     = []
}

variable "cors_origins_csv" {
  description = "Optional explicit CSV override for runtime CORS_ORIGINS."
  type        = string
  default     = ""
}

variable "ms_client_id" {
  description = "Microsoft OAuth client ID."
  type        = string
  default     = ""
}

variable "ms_client_secret" {
  description = "Microsoft OAuth client secret for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "ms_redirect_uri" {
  description = "Microsoft mail connect redirect URI."
  type        = string
  default     = ""
}

variable "ms_auth_redirect_uri" {
  description = "Microsoft auth redirect URI."
  type        = string
  default     = ""
}

variable "google_client_id" {
  description = "Google OAuth client ID."
  type        = string
  default     = ""
}

variable "google_client_secret" {
  description = "Google OAuth client secret for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "google_redirect_uri" {
  description = "Google OAuth redirect URI."
  type        = string
  default     = ""
}

variable "slack_mode" {
  description = "Slack integration mode."
  type        = string
  default     = "fake"
}

variable "slack_client_id" {
  description = "Slack OAuth client ID."
  type        = string
  default     = ""
}

variable "slack_client_secret" {
  description = "Slack OAuth client secret for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "slack_redirect_uri" {
  description = "Slack OAuth redirect URI."
  type        = string
  default     = ""
}

variable "encryption_key" {
  description = "Application encryption key for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "jwt_secret" {
  description = "Application JWT signing secret for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "auth_default_user_id" {
  description = "Fallback local-style user UUID consumed by the current service runtime."
  type        = string
  default     = "a0000001-0000-4000-8000-000000000001"
}

variable "bedrock_model_id" {
  description = "Bedrock model or inference profile identifier."
  type        = string
  default     = "eu.amazon.nova-2-lite-v1:0"
}

variable "bedrock_runtime_endpoint" {
  description = "Optional Bedrock runtime endpoint override for local Floci."
  type        = string
  default     = ""
}

variable "job_cursor_secret" {
  description = "Job list cursor signing secret for local/Floci only. Hosted envs use Secrets Manager."
  type        = string
  default     = ""
  sensitive   = true
}

variable "job_terminal_retention_days" {
  description = "Terminal transcript retention in days, applied via expires_at."
  type        = number
  default     = 30
}

variable "job_lease_seconds" {
  description = "Lease timeout for running jobs."
  type        = number
  default     = 960
}

variable "job_pending_wake_after_seconds" {
  description = "Delay before the scheduler re-wakes pending jobs."
  type        = number
  default     = 120
}

variable "jobs_inline" {
  description = "Whether to execute jobs inline instead of via DynamoDB streams."
  type        = bool
  default     = false
}

variable "run_migrations_on_deploy" {
  description = "Invoke the migrate Lambda after deployment."
  type        = bool
  default     = false
}

variable "cloudwatch_log_retention_days" {
  description = "Hosted CloudWatch log retention in days."
  type        = number
  default     = 30
}

variable "dsql_database_name" {
  description = "Hosted DSQL database name."
  type        = string
  default     = "postgres"
}

variable "dsql_runtime_role" {
  description = "Hosted runtime DSQL database role name."
  type        = string
  default     = "automata_runtime"
}

variable "dsql_admin_role" {
  description = "Hosted administrative DSQL database role name."
  type        = string
  default     = "admin"
}
