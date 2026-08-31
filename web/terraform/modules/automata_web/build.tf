resource "null_resource" "build_web" {
  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    interpreter = ["bash", "-c"]
    working_dir = "${path.module}/../../../"
    environment = {
      VITE_API_BASE_URL = var.api_base_url
    }
    command = <<-EOT
      set -euo pipefail
      if [[ -f package-lock.json ]]; then
        npm ci
      else
        npm install
      fi
      npm run build
    EOT
  }
}

resource "null_resource" "sync_to_s3" {
  depends_on = [
    aws_s3_bucket.website,
    aws_cloudfront_distribution.website,
    null_resource.build_web
  ]

  triggers = {
    build_hash = null_resource.build_web.id
  }

  provisioner "local-exec" {
    command = <<-EOT
      aws s3 sync "${path.module}/../../../dist" "s3://${aws_s3_bucket.website.bucket}" \
        --delete \
        --cache-control "public, max-age=31536000, immutable" \
        --exclude "index.html" \
        --exclude "*.html"

      aws s3 sync "${path.module}/../../../dist" "s3://${aws_s3_bucket.website.bucket}" \
        --cache-control "public, max-age=0, must-revalidate" \
        --exclude "*" \
        --include "index.html" \
        --include "*.html"
    EOT
  }
}

resource "null_resource" "cloudfront_invalidation" {
  depends_on = [null_resource.sync_to_s3]

  triggers = {
    sync_hash = null_resource.sync_to_s3.id
  }

  provisioner "local-exec" {
    command = "aws cloudfront create-invalidation --distribution-id ${aws_cloudfront_distribution.website.id} --paths '/*'"
  }
}
