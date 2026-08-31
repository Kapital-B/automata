terraform {
  required_version = ">= 1.10.3, < 2.0.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 6.15, < 7.0"
    }
    null = {
      source  = "hashicorp/null"
      version = ">= 3.2.0"
    }
  }

  backend "s3" {}
}

variable "cloudfront_price_class" {
  description = "CloudFront price class."
  type        = string
  default     = "PriceClass_100"
}

locals {
  environment = "prod"
  aws_region  = "us-east-1"
  # SPA apex zone is created in this account; API zone is owned by the backend account.
  hosted_zone_name = "automata.kapital-b.com"
  site_domain_name = "automata.kapital-b.com"
  api_base_url     = "https://api.automata.kapital-b.com"
}

provider "aws" {
  region = local.aws_region
}

module "automata_web" {
  source = "../../modules/automata_web"

  environment            = local.environment
  aws_region             = local.aws_region
  hosted_zone_name       = local.hosted_zone_name
  site_domain_name       = local.site_domain_name
  api_base_url           = local.api_base_url
  cloudfront_price_class = var.cloudfront_price_class
}

output "cloudfront_url" {
  description = "CloudFront URL."
  value       = "https://${module.automata_web.cloudfront_domain_name}"
}

output "hosted_zone_name" {
  description = "SPA Route53 hosted zone name (frontend account)."
  value       = module.automata_web.hosted_zone_name
}

output "hosted_zone_name_servers" {
  description = "Nameservers to NS-delegate for automata.kapital-b.com from the parent kapital-b.com zone."
  value       = module.automata_web.hosted_zone_name_servers
}
