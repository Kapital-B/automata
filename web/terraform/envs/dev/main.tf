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

  backend "s3" {
    bucket         = "automata-web-dev-terraform-state"
    key            = "automata_web.tfstate"
    region         = "us-east-1"
    dynamodb_table = "automata_web-lock-table"
  }
}

variable "cloudfront_price_class" {
  description = "CloudFront price class."
  type        = string
  default     = "PriceClass_100"
}

locals {
  environment = "dev"
  aws_region  = "us-east-1"
  # SPA apex zone is created in this account; API zone is owned by the backend account.
  hosted_zone_name = "automata-dev.kapital-b.com"
  site_domain_name = "automata-dev.kapital-b.com"
  api_base_url     = "https://api.automata-dev.kapital-b.com"
  # From: terraform -chdir=svc/terraform/envs/dev output -json hosted_zone_name_servers
  # Must be published here (not only on kapital-b.com) because api.* is under this apex.
  api_zone_name_servers = [
    "ns-122.awsdns-15.com.",
    "ns-960.awsdns-56.net.",
    "ns-1044.awsdns-02.org.",
    "ns-1571.awsdns-04.co.uk.",
  ]
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
  api_zone_name_servers  = local.api_zone_name_servers
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
  description = "Nameservers to NS-delegate for automata-dev.kapital-b.com from the parent kapital-b.com zone."
  value       = module.automata_web.hosted_zone_name_servers
}
