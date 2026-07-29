variable "team_name" {
  type        = string
  description = "Team/Tenant name"
}

variable "app_name" {
  type        = string
  description = "Application name"
}

provider "random" {}

resource "random_string" "bucket_suffix" {
  length  = 4
  lower   = true
  special = false
}

resource "aws_s3_bucket" "example" {
  bucket = "${var.team_name}-${var.app_name}-bucket-${random_string.bucket_suffix.result}"

  tags = {
    Name      = "${var.team_name}-${var.app_name}-bucket"
    Team      = var.team_name
    Service   = var.app_name
    ManagedBy = "terraform"
    Owner     = var.team_name
  }
}

resource "aws_s3_bucket_public_access_block" "example_public_access_block" {
  bucket = aws_s3_bucket.example.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "example_sse" {
  bucket = aws_s3_bucket.example.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}
