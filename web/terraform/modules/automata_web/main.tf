data "aws_caller_identity" "current" {}

data "aws_region" "current" {}

locals {
  bucket_name = "automata-web-${var.environment}-${data.aws_caller_identity.current.account_id}-${data.aws_region.current.region}"

  common_tags = {
    Environment = var.environment
    Project     = "automata"
    Component   = "web"
    ManagedBy   = "terraform"
  }
}

resource "aws_route53_zone" "public" {
  name = var.hosted_zone_name

  tags = merge(local.common_tags, {
    Name = var.hosted_zone_name
  })
}
