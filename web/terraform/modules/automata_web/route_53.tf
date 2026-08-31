resource "aws_route53_record" "website" {
  zone_id = aws_route53_zone.public.zone_id
  name    = var.site_domain_name
  type    = "A"

  alias {
    name                   = aws_cloudfront_distribution.website.domain_name
    zone_id                = aws_cloudfront_distribution.website.hosted_zone_id
    evaluate_target_health = false
  }
}
