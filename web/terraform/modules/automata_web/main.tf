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

# api.<apex> is hierarchically under this SPA zone. Resolvers that follow the
# apex delegation never see an NS cut published only on kapital-b.com, so they
# NXDOMAIN api.* and the browser never reaches API Gateway/Lambda.
resource "aws_route53_record" "api_zone_ns" {
  count = length(var.api_zone_name_servers) > 0 ? 1 : 0

  zone_id = aws_route53_zone.public.zone_id
  name    = "api"
  type    = "NS"
  ttl     = 300
  records = var.api_zone_name_servers
}
