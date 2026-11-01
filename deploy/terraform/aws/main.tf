terraform {
  required_version = ">= 1.5.0"
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

# 1. Customer-Managed KMS Key for Envelope Encryption
resource "aws_kms_key" "clm_master_key" {
  description             = "Customer In-VPC KMS Master Key for CLM Envelope Encryption"
  deletion_window_in_days = 30
  enable_key_rotation     = true

  tags = {
    Environment = var.environment
    Component   = "CLM-Satellite-Enclave"
    Compliance  = "FIPS-140-3-Level-3"
  }
}

resource "aws_kms_alias" "clm_master_key_alias" {
  name          = "alias/clm-customer-signing-master"
  target_key_id = aws_kms_key.clm_master_key.key_id
}

# 2. Immutable WORM Storage S3 Bucket for Contract Evidence Bundles
resource "aws_s3_bucket" "clm_evidence_vault" {
  bucket        = "enterprise-clm-evidence-${var.environment}"
  force_destroy = false

  object_lock_configuration {
    object_lock_enabled = "Enabled"
  }

  tags = {
    Environment = var.environment
    Compliance  = "WORM-SOX-ISO27001"
  }
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
  identifier            = "clm-db-${var.environment}"
  allocated_storage     = 100
  max_allocated_storage = 1000
  storage_type          = "gp3"
  engine                = "postgres"
  engine_version        = "15.6"
  instance_class        = "db.r6g.xlarge"
  multi_az              = true
  db_name               = "clmdb"
  username              = "clm_master_admin"
  password              = "CHANGE_ME_IN_VAULT"
  storage_encrypted     = true
  kms_key_id            = aws_kms_key.clm_master_key.arn
  skip_final_snapshot   = false
  backup_retention_period = 35
  deletion_protection   = true

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

