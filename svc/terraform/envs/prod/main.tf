terraform {
  required_version = ">= 1.10.3, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.15, < 7.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = ">= 2.5.0"
    }
    null = {
      source  = "hashicorp/null"
      version = ">= 3.2.0"
    }
  }

  backend "s3" {}
}

variable "go_arch" {
  description = "GOARCH for CI and hosted Lambda builds."
  type        = string
  default     = "amd64"
}

variable "lambda_architecture" {
  description = "Lambda architecture name matching go_arch."
  type        = string
  default     = "x86_64"
}

variable "ms_client_id" {
  description = "Microsoft OAuth client ID (public)."
  type        = string
}

variable "google_client_id" {
  description = "Optional Google OAuth client ID (public)."
  type        = string
  default     = ""
}

variable "slack_mode" {
  description = "Slack integration mode."
  type        = string
  default     = "oauth"
}

variable "slack_client_id" {
  description = "Optional Slack OAuth client ID (public)."
  type        = string
  default     = ""
}

variable "auth_default_user_id" {
  description = "Fallback UUID consumed by the current auth middleware."
  type        = string
  default     = "a0000001-0000-4000-8000-000000000001"
}

locals {
  environment = "prod"
  aws_region  = "eu-west-2"
  # API zone lives in this account; SPA apex zone lives in the frontend account.
  hosted_zone_name     = "api.automata.kapital-b.com"
  api_domain_name      = "api.automata.kapital-b.com"
  dashboard_base_url   = "https://automata.kapital-b.com"
  ms_redirect_uri      = "https://api.automata.kapital-b.com/api/accounts/callback"
  ms_auth_redirect_uri = "https://api.automata.kapital-b.com/api/auth/microsoft/callback"
  google_redirect_uri  = "https://api.automata.kapital-b.com/api/auth/google/callback"
  slack_redirect_uri   = "https://api.automata.kapital-b.com/api/connectors/callback"
  cors_origins = [
    "https://automata.kapital-b.com"
  ]
}

provider "aws" {
  region = local.aws_region
}

module "automata" {
  source = "../../modules/automata"

  environment                   = local.environment
  enable_hosted                 = true
  aws_region                    = local.aws_region
  go_arch                       = var.go_arch
  lambda_architecture           = var.lambda_architecture
  hosted_zone_name              = local.hosted_zone_name
  api_domain_name               = local.api_domain_name
  app_public_url                = "https://${local.api_domain_name}"
  dashboard_base_url            = local.dashboard_base_url
  cors_origins                  = local.cors_origins
  ms_client_id                  = var.ms_client_id
  ms_redirect_uri               = local.ms_redirect_uri
  ms_auth_redirect_uri          = local.ms_auth_redirect_uri
  google_client_id              = var.google_client_id
  google_redirect_uri           = local.google_redirect_uri
  slack_mode                    = var.slack_mode
  slack_client_id               = var.slack_client_id
  slack_redirect_uri            = local.slack_redirect_uri
  auth_default_user_id          = var.auth_default_user_id
  bedrock_model_id              = "eu.amazon.nova-2-lite-v1:0"
  job_terminal_retention_days   = 30
  cloudwatch_log_retention_days = 30
  run_migrations_on_deploy      = true
}

output "api_gateway_url" {
  description = "API Gateway invoke URL."
  value       = module.automata.api_gateway_url
}

output "api_custom_domain_name" {
  description = "API custom domain."
  value       = module.automata.api_custom_domain_name
}

output "hosted_zone_name_servers" {
  description = "Nameservers to NS-delegate for api.automata.kapital-b.com from the parent kapital-b.com zone."
  value       = module.automata.hosted_zone_name_servers
}

output "hosted_zone_name" {
  description = "API Route53 hosted zone name (backend account)."
  value       = local.hosted_zone_name
}

output "dsql_cluster_endpoint" {
  description = "Hosted DSQL cluster endpoint."
  value       = module.automata.dsql_cluster_endpoint
}

output "secret_names" {
  description = "Secrets Manager names to populate manually after apply."
  value       = module.automata.secret_names
}
