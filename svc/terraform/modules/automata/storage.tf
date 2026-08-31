resource "aws_s3_bucket" "stream_failures" {
  bucket = "automata-stream-failures-${var.environment}"
  tags   = local.common_tags
}

resource "aws_s3_bucket_public_access_block" "stream_failures" {
  bucket                  = aws_s3_bucket.stream_failures.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "stream_failures" {
  bucket = aws_s3_bucket.stream_failures.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "stream_failures" {
  bucket = aws_s3_bucket.stream_failures.id

  rule {
    id     = "retention"
    status = "Enabled"

    filter {}

    expiration {
      days = 90
    }
  }
}
