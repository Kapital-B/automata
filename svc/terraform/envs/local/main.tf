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
    external = {
      source  = "hashicorp/external"
      version = ">= 2.3.0"
    }
    null = {
      source  = "hashicorp/null"
      version = ">= 3.2.0"
    }
  }

  backend "local" {}
}

locals {
  environment              = "local"
  aws_region               = "us-east-1"
  lambda_runtime_endpoint  = "http://floci:4566"
  lambda_database_url      = "postgres://automata:automata@postgres:5432/automata?sslmode=disable"
  default_cors_origins_csv = "http://localhost:5173,http://127.0.0.1:5173"
}

provider "aws" {
  region                      = local.aws_region
  access_key                  = "test"
  secret_key                  = "test"
  skip_credentials_validation = true
  skip_metadata_api_check     = true
  skip_requesting_account_id  = true
  s3_use_path_style           = true

  endpoints {
    apigateway     = "http://localhost:4566"
    dynamodb       = "http://localhost:4566"
    events         = "http://localhost:4566"
    iam            = "http://localhost:4566"
    lambda         = "http://localhost:4566"
    s3             = "http://localhost:4566"
    secretsmanager = "http://localhost:4566"
    sts            = "http://localhost:4566"
  }
}

module "automata" {
  source = "../../modules/automata"

  environment                    = local.environment
  aws_region                     = local.aws_region
  aws_endpoint                   = local.lambda_runtime_endpoint
  database_engine                = "postgres"
  database_url                   = local.lambda_database_url
  lambda_architecture            = var.lambda_architecture
  app_public_url                 = var.app_public_url
  dashboard_base_url             = var.dashboard_base_url
  cors_origins_csv               = var.cors_origins_csv != "" ? var.cors_origins_csv : local.default_cors_origins_csv
  ms_client_id                   = var.ms_client_id
  ms_client_secret               = var.ms_client_secret
  ms_redirect_uri                = var.ms_redirect_uri
  ms_auth_redirect_uri           = var.ms_auth_redirect_uri
  google_client_id               = var.google_client_id
  google_client_secret           = var.google_client_secret
  google_redirect_uri            = var.google_redirect_uri
  slack_mode                     = var.slack_mode
  slack_client_id                = var.slack_client_id
  slack_client_secret            = var.slack_client_secret
  slack_redirect_uri             = var.slack_redirect_uri
  encryption_key                 = var.encryption_key
  jwt_secret                     = var.jwt_secret
  auth_default_user_id           = var.auth_default_user_id
  bedrock_model_id               = var.bedrock_model_id
  bedrock_runtime_endpoint       = var.bedrock_runtime_endpoint != "" ? var.bedrock_runtime_endpoint : local.lambda_runtime_endpoint
  job_cursor_secret              = var.job_cursor_secret != "" ? var.job_cursor_secret : var.jwt_secret
  job_terminal_retention_days    = var.job_terminal_retention_days
  job_lease_seconds              = var.job_lease_seconds
  job_pending_wake_after_seconds = var.job_pending_wake_after_seconds
  jobs_inline                    = false
}
