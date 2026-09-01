variable "environment" {
  description = "Deployment environment name."
  type        = string
}

variable "aws_region" {
  description = "AWS region for the ACM and CloudFront deployment workspace."
  type        = string
  default     = "us-east-1"
}

variable "hosted_zone_name" {
  description = "SPA apex hosted zone created in this account (e.g. automata-dev.kapital-b.com). API zone is owned by the backend account."
  type        = string
}

variable "site_domain_name" {
  description = "Apex domain name for the SPA."
  type        = string
}

variable "api_base_url" {
  description = "Baked VITE_API_BASE_URL value for the SPA build."
  type        = string
}

variable "api_zone_name_servers" {
  description = "NS records for api.<spa-apex> hosted in the backend account. Required because api.* is a child of the SPA apex zone."
  type        = list(string)
  default     = []
}

variable "cloudfront_price_class" {
  description = "CloudFront price class."
  type        = string
  default     = "PriceClass_100"
}
