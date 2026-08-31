# Hosted secrets are created empty (or with a REPLACE-ME placeholder). Populate
# values manually in Secrets Manager after apply; Terraform never owns the value.

resource "aws_secretsmanager_secret" "encryption_key" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/ENCRYPTION_KEY"
  description = "32-byte application encryption key. Populate manually after apply."

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "jwt_secret" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/JWT_SECRET"
  description = "JWT signing secret (>=32 bytes). Populate manually after apply."

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "job_cursor_secret" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/JOB_CURSOR_SECRET"
  description = "Job list cursor signing secret (>=32 bytes). Populate manually after apply; may match JWT_SECRET."

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "ms_client_secret" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/MS_CLIENT_SECRET"
  description = "Microsoft OAuth client secret. Populate manually after apply."

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "google_client_secret" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/GOOGLE_CLIENT_SECRET"
  description = "Google OAuth client secret. Populate manually after apply when Google auth is enabled."

  tags = local.common_tags
}

resource "aws_secretsmanager_secret" "slack_client_secret" {
  count = local.enable_hosted ? 1 : 0

  name        = "automata/${var.environment}/SLACK_CLIENT_SECRET"
  description = "Slack OAuth client secret. Populate manually after apply when Slack OAuth is enabled."

  tags = local.common_tags
}

locals {
  hosted_secret_arns = local.enable_hosted ? compact([
    aws_secretsmanager_secret.encryption_key[0].arn,
    aws_secretsmanager_secret.jwt_secret[0].arn,
    aws_secretsmanager_secret.job_cursor_secret[0].arn,
    aws_secretsmanager_secret.ms_client_secret[0].arn,
    aws_secretsmanager_secret.google_client_secret[0].arn,
    aws_secretsmanager_secret.slack_client_secret[0].arn,
  ]) : []

  hosted_secret_env = local.enable_hosted ? {
    ENCRYPTION_KEY_SECRET_ID       = aws_secretsmanager_secret.encryption_key[0].name
    JWT_SECRET_SECRET_ID           = aws_secretsmanager_secret.jwt_secret[0].name
    JOB_CURSOR_SECRET_SECRET_ID    = aws_secretsmanager_secret.job_cursor_secret[0].name
    MS_CLIENT_SECRET_SECRET_ID     = aws_secretsmanager_secret.ms_client_secret[0].name
    GOOGLE_CLIENT_SECRET_SECRET_ID = aws_secretsmanager_secret.google_client_secret[0].name
    SLACK_CLIENT_SECRET_SECRET_ID  = aws_secretsmanager_secret.slack_client_secret[0].name
  } : {}

  local_secret_env = local.enable_hosted ? {} : {
    ENCRYPTION_KEY       = var.encryption_key
    JWT_SECRET           = var.jwt_secret
    JOB_CURSOR_SECRET    = var.job_cursor_secret
    MS_CLIENT_SECRET     = var.ms_client_secret
    GOOGLE_CLIENT_SECRET = var.google_client_secret
    SLACK_CLIENT_SECRET  = var.slack_client_secret
  }
}
