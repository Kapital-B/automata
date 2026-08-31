data "aws_caller_identity" "current" {}

data "aws_partition" "current" {}

resource "aws_dsql_cluster" "hosted" {
  count = local.enable_hosted ? 1 : 0

  deletion_protection_enabled = true

  tags = merge(local.common_tags, {
    name = "automata-dsql-${var.environment}"
  })
}

resource "aws_route53_zone" "public" {
  count = local.enable_hosted ? 1 : 0

  name = var.hosted_zone_name

  tags = merge(local.common_tags, {
    name = var.hosted_zone_name
  })
}

resource "aws_acm_certificate" "api" {
  count = local.enable_hosted ? 1 : 0

  domain_name       = var.api_domain_name
  validation_method = "DNS"

  lifecycle {
    create_before_destroy = true
  }

  tags = merge(local.common_tags, {
    name = var.api_domain_name
  })
}

resource "aws_route53_record" "api_cert_validation" {
  for_each = local.enable_hosted ? {
    for dvo in aws_acm_certificate.api[0].domain_validation_options : dvo.domain_name => {
      name   = dvo.resource_record_name
      record = dvo.resource_record_value
      type   = dvo.resource_record_type
    }
  } : {}

  allow_overwrite = true
  zone_id         = aws_route53_zone.public[0].zone_id
  name            = each.value.name
  type            = each.value.type
  ttl             = 60
  records         = [each.value.record]
}

resource "aws_acm_certificate_validation" "api" {
  count = local.enable_hosted ? 1 : 0

  certificate_arn         = aws_acm_certificate.api[0].arn
  validation_record_fqdns = [for record in aws_route53_record.api_cert_validation : record.fqdn]
}

resource "aws_api_gateway_domain_name" "api" {
  count = local.enable_hosted ? 1 : 0

  domain_name              = var.api_domain_name
  regional_certificate_arn = aws_acm_certificate_validation.api[0].certificate_arn

  endpoint_configuration {
    types = ["REGIONAL"]
  }

  tags = merge(local.common_tags, {
    name = var.api_domain_name
  })
}

resource "aws_api_gateway_base_path_mapping" "api" {
  count = local.enable_hosted ? 1 : 0

  api_id      = aws_api_gateway_rest_api.api.id
  stage_name  = aws_api_gateway_stage.api.stage_name
  domain_name = aws_api_gateway_domain_name.api[0].domain_name
}

resource "aws_route53_record" "api_alias" {
  count = local.enable_hosted ? 1 : 0

  zone_id = aws_route53_zone.public[0].zone_id
  name    = var.api_domain_name
  type    = "A"

  alias {
    name                   = aws_api_gateway_domain_name.api[0].regional_domain_name
    zone_id                = aws_api_gateway_domain_name.api[0].regional_zone_id
    evaluate_target_health = true
  }
}

resource "aws_cloudwatch_log_group" "api" {
  count = local.enable_hosted ? 1 : 0

  name              = "/aws/lambda/${aws_lambda_function.api.function_name}"
  retention_in_days = var.cloudwatch_log_retention_days
  tags              = local.common_tags
}

resource "aws_cloudwatch_log_group" "scheduler" {
  count = local.enable_hosted ? 1 : 0

  name              = "/aws/lambda/${aws_lambda_function.scheduler.function_name}"
  retention_in_days = var.cloudwatch_log_retention_days
  tags              = local.common_tags
}

resource "aws_cloudwatch_log_group" "worker" {
  count = local.enable_hosted ? 1 : 0

  name              = "/aws/lambda/${aws_lambda_function.worker.function_name}"
  retention_in_days = var.cloudwatch_log_retention_days
  tags              = local.common_tags
}

resource "aws_cloudwatch_log_group" "migrate" {
  count = local.enable_hosted ? 1 : 0

  name              = "/aws/lambda/${aws_lambda_function.migrate.function_name}"
  retention_in_days = var.cloudwatch_log_retention_days
  tags              = local.common_tags
}
