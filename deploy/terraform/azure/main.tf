terraform {
  required_version = ">= 1.5.0"
  backend "azurerm" {
    use_azuread_auth = true
  }

  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.90"
    }
  }
}

provider "azurerm" {
  features {}
}

variable "location" {
  type    = string
  default = "East US"
}

variable "resource_group_name" {
  type    = string
  default = "rg-clm-enterprise-prod"
}

variable "administrator_password" {
  description = "PostgreSQL administrator password; supply securely and protect Terraform state."
  type        = string
  sensitive   = true
  nullable    = false

  validation {
    condition     = length(var.administrator_password) >= 16 && length(var.administrator_password) <= 128
    error_message = "administrator_password must be between 16 and 128 characters."
  }
}

# 1. Resource Group
resource "azurerm_resource_group" "clm_rg" {
  name     = var.resource_group_name
  location = var.location
}

# 2. Azure Key Vault for In-VPC Customer Key Custody
data "azurerm_client_config" "current" {}

resource "azurerm_key_vault" "clm_vault" {
  name                       = "kv-clm-prod-001"
  location                   = azurerm_resource_group.clm_rg.location
  resource_group_name        = azurerm_resource_group.clm_rg.name
  tenant_id                  = data.azurerm_client_config.current.tenant_id
  sku_name                   = "premium" # HSM backed keys
  soft_delete_retention_days = 90
  purge_protection_enabled   = true

  network_acls {
    bypass         = "AzureServices"
    default_action = "Deny"
  }
}

resource "azurerm_key_vault_key" "clm_signing_key" {
  name         = "clm-customer-signing-key"
  key_vault_id = azurerm_key_vault.clm_vault.id
  key_type     = "RSA-HSM"
  key_size     = 4096

  key_opts = [
    "decrypt",
    "encrypt",
    "sign",
    "unwrapKey",
    "verify",
    "wrapKey",
  ]
}

# 3. Azure Database for PostgreSQL Flexible Server
resource "azurerm_postgresql_flexible_server" "clm_pg" {
  name                         = "psql-clm-prod-flexible"
  resource_group_name          = azurerm_resource_group.clm_rg.name
  location                     = azurerm_resource_group.clm_rg.location
  version                      = "15"
  administrator_login          = "clm_admin"
  administrator_password       = var.administrator_password
  storage_mb                   = 131072
  sku_name                     = "GP_Standard_D4ds_v4"
  backup_retention_days        = 35
  geo_redundant_backup_enabled = true

  high_availability {
    mode = "ZoneRedundant"
  }
}
