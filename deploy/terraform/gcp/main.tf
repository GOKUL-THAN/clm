terraform {
  required_version = ">= 1.5.0"
  backend "gcs" {}

  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.20"
    }
  }
}

variable "project_id" {
  type    = string
  default = "clm-enterprise-production"
}

variable "region" {
  type    = string
  default = "us-central1"
}

provider "google" {
  project = var.project_id
  region  = var.region
}

# 1. Cloud KMS KeyRing & HSM Master Key
resource "google_kms_key_ring" "clm_keyring" {
  name     = "clm-satellite-keyring"
  location = var.region
}

resource "google_kms_crypto_key" "clm_crypto_key" {
  name            = "clm-envelope-signing-key"
  key_ring        = google_kms_key_ring.clm_keyring.id
  rotation_period = "7776000s" # 90 days

  version_template {
    algorithm        = "GOOGLE_SYMMETRIC_ENCRYPTION"
    protection_level = "HSM"
  }
}

# 2. Cloud SQL for PostgreSQL 15 HA
resource "google_sql_database_instance" "clm_db" {
  name             = "clm-postgres-prod-instance"
  database_version = "POSTGRES_15"
  region           = var.region

  settings {
    tier              = "db-custom-8-32768"
    availability_type = "REGIONAL"

    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      start_time                     = "03:00"
      transaction_log_retention_days = 7
    }

    ip_configuration {
      ipv4_enabled     = false
      private_network = "projects/${var.project_id}/global/networks/clm-vpc"
      require_ssl     = true
    }
  }

  deletion_protection = true
}
