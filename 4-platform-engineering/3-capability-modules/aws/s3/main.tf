terraform {
  required_version = ">= 1.5"

  # Declared here, NOT as a `provider "random" {}` block. A module that declares
  # its own provider configuration cannot be used with count/for_each and cannot
  # be cleanly removed — Terraform requires the config to outlive the resources.
  # Providers are configured once in the root module (platform/providers.tf).
  required_providers {
    aws    = { source = "hashicorp/aws", version = "~> 6.0" }
    random = { source = "hashicorp/random", version = "~> 3.6" }
  }
}

variable "team_name" {
  type        = string
  description = "Team/tenant that owns this bucket"
}

variable "app_name" {
  type        = string
  description = "Application the bucket belongs to"
}

variable "env" {
  type        = string
  description = "Target environment (dev, staging, prod)"
  default     = "dev"
}

variable "tags" {
  type        = map(string)
  description = "Tags applied to every resource, supplied by the scaffolder"
  default     = {}
}

# S3 bucket names live in one global namespace shared by every AWS account on
# earth, so "payments-checkout" is almost certainly taken. The suffix makes a
# collision improbable without asking humans to invent unique names.
resource "random_string" "bucket_suffix" {
  length = 6

  # upper MUST be false: S3 bucket names are lowercase-only, and random_string
  # defaults `upper` to true. Setting only `lower = true` (the previous version)
  # is a no-op — lower is already the default, and it does not exclude uppercase.
  # That produced invalid bucket names roughly half the time.
  upper   = false
  special = false
}

locals {
  # Bucket names are 3-63 characters. team_name and app_name are each capped at
  # 40 by the scaffolder's input schema, so the prefix can overflow on its own;
  # truncate before appending the 6-char suffix. (locals inside a module have
  # their own namespace, so this cannot collide with another capability's file.)
  raw_prefix  = lower("${var.team_name}-${var.app_name}-${var.env}")
  bucket_name = "${substr(local.raw_prefix, 0, 56)}-${random_string.bucket_suffix.result}"
}

resource "aws_s3_bucket" "this" {
  bucket = local.bucket_name

  tags = merge(var.tags, {
    Name  = local.bucket_name
    Owner = var.team_name
  })
}

# The guardrails below are not optional knobs. A tenant cannot opt out of them
# from the scaffolder, which is the whole argument for a platform module over
# letting each team write raw aws_s3_bucket resources.

resource "aws_s3_bucket_public_access_block" "this" {
  bucket = aws_s3_bucket.this.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "this" {
  bucket = aws_s3_bucket.this.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Versioning protects against the most common data-loss incident there is: an
# application bug or a bad deploy overwriting objects. Cheap; on by default.
resource "aws_s3_bucket_versioning" "this" {
  bucket = aws_s3_bucket.this.id

  versioning_configuration {
    status = "Enabled"
  }
}

output "bucket_name" {
  description = "Generated bucket name, including the uniqueness suffix"
  value       = aws_s3_bucket.this.id
}

output "bucket_arn" {
  description = "Bucket ARN, for IAM policies written elsewhere"
  value       = aws_s3_bucket.this.arn
}
