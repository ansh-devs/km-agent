# =============================================================================
# KloudMate Agent - Terraform User Data Module
# =============================================================================
# Usage in any cloud provider's VM resource:
#
#   module "kmagent_userdata" {
#     source       = "./modules/user-data"
#     km_api_key   = var.km_api_key
#     km_endpoint  = "https://ingest.kloudmate.com"
#     kmagent_tags = { env = "production", team = "infra" }
#   }
#
#   # AWS
#   resource "aws_launch_template" "app" {
#     user_data = base64encode(module.kmagent_userdata.cloud_init)
#   }
#
#   # GCP
#   resource "google_compute_instance_template" "app" {
#     metadata = { user-data = module.kmagent_userdata.cloud_init }
#   }
#
#   # Azure
#   resource "azurerm_linux_virtual_machine_scale_set" "app" {
#     custom_data = base64encode(module.kmagent_userdata.cloud_init)
#   }
#
#   # DigitalOcean
#   resource "digitalocean_droplet" "app" {
#     user_data = module.kmagent_userdata.cloud_init
#   }
#
#   # OCI
#   resource "oci_core_instance" "app" {
#     metadata = {
#       user_data = base64encode(module.kmagent_userdata.cloud_init)
#     }
#   }
# =============================================================================

variable "km_api_key" {
  description = "KloudMate API key"
  type        = string
  sensitive   = true
}

variable "km_endpoint" {
  description = "KloudMate ingest endpoint"
  type        = string
  default     = "https://ingest.kloudmate.com"
}

variable "kmagent_version" {
  description = "Agent version to install (or 'latest')"
  type        = string
  default     = "latest"
}

variable "kmagent_tags" {
  description = "Key-value tags to attach to the agent"
  type        = map(string)
  default     = {}
}

variable "kmagent_install_script_url" {
  description = "URL to the install script"
  type        = string
  default     = "https://raw.githubusercontent.com/kloudmate/km-agent/main/install.sh"
}

locals {
  tags_args = join(" ", [for k, v in var.kmagent_tags : "--tag ${k}=${v}"])
}

output "cloud_init" {
  description = "cloud-init YAML to pass as user_data / custom_data"
  value = <<-CLOUDINIT
    #cloud-config
    package_update: true
    packages: [curl, jq]
    runcmd:
      - |
        curl -fsSL ${var.kmagent_install_script_url} \
          | bash -s -- \
            --api-key "${var.km_api_key}" \
            --endpoint "${var.km_endpoint}" \
            ${var.kmagent_version != "latest" ? "--version ${var.kmagent_version}" : ""} \
            ${local.tags_args}
      - |
        if systemctl is-active --quiet kmagent; then
          echo "[kloudmate] Agent running"
        else
          echo "[kloudmate] ERROR: Agent failed to start"
        fi
  CLOUDINIT
}

output "shell_script" {
  description = "Plain bash script (for providers that don't support cloud-init)"
  value = <<-BASH
    #!/bin/bash
    set -euo pipefail
    curl -fsSL ${var.kmagent_install_script_url} \
      | bash -s -- \
        --api-key "${var.km_api_key}" \
        --endpoint "${var.km_endpoint}" \
        ${var.kmagent_version != "latest" ? "--version ${var.kmagent_version}" : ""} \
        ${local.tags_args}
  BASH
}
