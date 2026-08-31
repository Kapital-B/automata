output "cloudfront_domain_name" {
  description = "CloudFront distribution hostname."
  value       = aws_cloudfront_distribution.website.domain_name
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution identifier."
  value       = aws_cloudfront_distribution.website.id
}

output "cloudfront_zone_id" {
  description = "CloudFront hosted zone identifier."
  value       = aws_cloudfront_distribution.website.hosted_zone_id
}

output "s3_bucket_name" {
  description = "SPA bucket name."
  value       = aws_s3_bucket.website.bucket
}

output "hosted_zone_name" {
  description = "SPA Route53 hosted zone name (frontend account)."
  value       = aws_route53_zone.public.name
}

output "hosted_zone_name_servers" {
  description = "Nameservers to NS-delegate for the SPA apex from the parent kapital-b.com zone."
  value       = aws_route53_zone.public.name_servers
}
