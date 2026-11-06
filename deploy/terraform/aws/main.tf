terraform {
  required_version = ">= 1.10.0"
  backend "s3" {
    encrypt      = true
    use_lockfile = true
  }

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.40"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "environment" {
  type    = string
  default = "production"
}

variable "evidence_bucket_name" {
  description = "Globally unique S3 bucket name for retained CLM evidence."
  type        = string
  nullable    = false

  validation {
    condition     = length(var.evidence_bucket_name) >= 3 && length(var.evidence_bucket_name) <= 63
    error_message = "evidence_bucket_name must be a valid-length S3 bucket name."
  }
}

variable "evidence_retention_days" {
  description = "Evidence retention duration required by the approved retention policy."
  type        = number
  nullable    = false

  validation {
    condition     = var.evidence_retention_days >= 1 && var.evidence_retention_days <= 36500
    error_message = "evidence_retention_days must be between 1 and 36500."
  }
}

variable "evidence_retention_mode" {
  description = "Approved S3 Object Lock mode; select COMPLIANCE only after legal and operational review."
  type        = string
  nullable    = false

  validation {
    condition     = contains(["GOVERNANCE", "COMPLIANCE"], upper(var.evidence_retention_mode))
    error_message = "evidence_retention_mode must be GOVERNANCE or COMPLIANCE."
  }
}

# 1. Customer-Managed KMS Key for Envelope Encryption
resource "aws_kms_key" "clm_master_key" {
  description             = "Customer In-VPC KMS Master Key for CLM Envelope Encryption"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = {
    Environment = var.environment
    Component   = "CLM-Satellite-Enclave"
  }
}

resource "aws_kms_alias" "clm_master_key_alias" {
  name          = "alias/clm-customer-signing-master"
  target_key_id = aws_kms_key.clm_master_key.key_id
}

# 2. Immutable WORM Storage S3 Bucket for Contract Evidence Bundles
resource "aws_s3_bucket" "clm_evidence_vault" {
  bucket        = var.evidence_bucket_name
  force_destroy = false

  object_lock_configuration {
    object_lock_enabled = "Enabled"
    rule {
      default_retention {
        mode = upper(var.evidence_retention_mode)
        days = var.evidence_retention_days
      }
    }
  }

  tags = {
    Environment = var.environment
    Component   = "CLM-Evidence-Vault"
  }
}

resource "aws_s3_bucket_public_access_block" "clm_evidence_vault" {
  bucket                  = aws_s3_bucket.clm_evidence_vault.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "clm_evidence_vault" {
  bucket = aws_s3_bucket.clm_evidence_vault.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_ownership_controls" "clm_evidence_vault" {
  bucket = aws_s3_bucket.clm_evidence_vault.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

data "aws_iam_policy_document" "clm_evidence_vault" {
  statement {
    sid    = "DenyInsecureTransport"
    effect = "Deny"

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    actions   = ["s3:*"]
    resources = [aws_s3_bucket.clm_evidence_vault.arn, "${aws_s3_bucket.clm_evidence_vault.arn}/*"]

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "clm_evidence_vault" {
  bucket = aws_s3_bucket.clm_evidence_vault.id
  policy = data.aws_iam_policy_document.clm_evidence_vault.json
}

resource "aws_s3_bucket_server_side_encryption_configuration" "clm_evidence_enc" {
  bucket = aws_s3_bucket.clm_evidence_vault.id
  rule {
    apply_server_side_encryption_by_default {
      kms_master_key_id = aws_kms_key.clm_master_key.arn
      sse_algorithm     = "aws:kms"
    }
  }
}

# 3. High Availability RDS PostgreSQL Multi-AZ Instance
resource "aws_db_instance" "clm_rds" {
  identifier                  = "clm-db-${var.environment}"
  allocated_storage           = 100
  max_allocated_storage       = 1000
  storage_type                = "gp3"
  engine                      = "postgres"
  engine_version              = "15.6"
  instance_class              = "db.r6g.xlarge"
  multi_az                    = true
  db_name                     = "clmdb"
  username                    = "clm_master_admin"
  manage_master_user_password = true
  storage_encrypted           = true
  kms_key_id                  = aws_kms_key.clm_master_key.arn
  publicly_accessible         = false
  skip_final_snapshot         = false
  backup_retention_period     = 35
  deletion_protection         = true

  tags = {
    Environment = var.environment
    Component   = "CLM-PostgreSQL-Core"
  }
}

output "kms_key_arn" {
  value       = aws_kms_key.clm_master_key.arn
  description = "Customer In-VPC KMS Master Key ARN for Satellite Gateway"
}

output "rds_endpoint" {
  value       = aws_db_instance.clm_rds.endpoint
  description = "High Availability PostgreSQL Primary Endpoint"
}
