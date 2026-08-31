variable "lambda_architecture" {
  type    = string
  default = "amd64"
}

variable "app_public_url" {
  type    = string
  default = "http://localhost:4566"
}

variable "dashboard_base_url" {
  type    = string
  default = "http://localhost:5173"
}

variable "cors_origins_csv" {
  type    = string
  default = ""
}

variable "ms_client_id" {
  type    = string
  default = ""
}

variable "ms_client_secret" {
  type    = string
  default = ""
}

variable "ms_redirect_uri" {
  type    = string
  default = "http://localhost:4566/restapis/replace-after-deploy/local/_user_request_/api/accounts/callback"
}

variable "ms_auth_redirect_uri" {
  type    = string
  default = "http://localhost:4566/restapis/replace-after-deploy/local/_user_request_/api/auth/microsoft/callback"
}

variable "google_client_id" {
  type    = string
  default = ""
}

variable "google_client_secret" {
  type    = string
  default = ""
}

variable "google_redirect_uri" {
  type    = string
  default = "http://localhost:4566/restapis/replace-after-deploy/local/_user_request_/api/auth/google/callback"
}

variable "slack_mode" {
  type    = string
  default = "fake"
}

variable "slack_client_id" {
  type    = string
  default = ""
}

variable "slack_client_secret" {
  type    = string
  default = ""
}

variable "slack_redirect_uri" {
  type    = string
  default = "http://localhost:4566/restapis/replace-after-deploy/local/_user_request_/api/connectors/callback"
}

variable "encryption_key" {
  type    = string
  default = "12345678901234567890123456789012"
}

variable "jwt_secret" {
  type    = string
  default = "abcdefghijklmnopqrstuvwxyz123456"
}

variable "auth_default_user_id" {
  type    = string
  default = "a0000001-0000-4000-8000-000000000001"
}

variable "bedrock_model_id" {
  type    = string
  default = "eu.amazon.nova-2-lite-v1:0"
}

variable "bedrock_runtime_endpoint" {
  type    = string
  default = ""
}

variable "job_cursor_secret" {
  type    = string
  default = ""
}

variable "job_terminal_retention_days" {
  type    = number
  default = 30
}

variable "job_lease_seconds" {
  type    = number
  default = 960
}

variable "job_pending_wake_after_seconds" {
  type    = number
  default = 120
}
