# =============================================================================
# KloudMate Agent - Packer Multi-Cloud Image Builder
# =============================================================================
# Build images with kmagent pre-installed for faster instance boot times.
#
# Usage:
#   packer build -var 'km_api_key=YOUR_KEY' kmagent-image.pkr.hcl
#   packer build -only=amazon-ebs.kmagent kmagent-image.pkr.hcl    # AWS only
#   packer build -only=googlecompute.kmagent kmagent-image.pkr.hcl  # GCP only
# =============================================================================

packer {
  required_plugins {
    amazon = {
      version = ">= 1.2.0"
      source  = "github.com/hashicorp/amazon"
    }
    googlecompute = {
      version = ">= 1.1.0"
      source  = "github.com/hashicorp/googlecompute"
    }
    azure = {
      version = ">= 2.0.0"
      source  = "github.com/hashicorp/azure"
    }
    oci = {
      version = ">= 1.0.0"
      source  = "github.com/hashicorp/oci"
    }
  }
}

# --- Variables ---
variable "kmagent_version" {
  type    = string
  default = "latest"
}

variable "base_ami" {
  type        = string
  default     = "ami-0c7217cdde317cfec" # Ubuntu 22.04 us-east-1
  description = "Base AMI for AWS"
}

variable "gcp_project" {
  type    = string
  default = ""
}

variable "gcp_zone" {
  type    = string
  default = "us-central1-a"
}

variable "azure_subscription_id" {
  type    = string
  default = ""
}

variable "azure_resource_group" {
  type    = string
  default = ""
}

# --- OCI Variables ---
variable "oci_tenancy_ocid" {
  type    = string
  default = ""
}

variable "oci_user_ocid" {
  type    = string
  default = ""
}

variable "oci_fingerprint" {
  type    = string
  default = ""
}

variable "oci_key_file" {
  type    = string
  default = ""
}

variable "oci_region" {
  type    = string
  default = "us-ashburn-1"
}

variable "oci_compartment_ocid" {
  type    = string
  default = ""
}

variable "oci_subnet_ocid" {
  type    = string
  default = ""
}

# --- AWS Source ---
source "amazon-ebs" "kmagent" {
  ami_name      = "kloudmate-agent-{{timestamp}}"
  instance_type = "t3.micro"
  region        = "us-east-1"
  source_ami    = var.base_ami

  ssh_username = "ubuntu"

  tags = {
    Name        = "kloudmate-agent"
    BuildTime   = "{{timestamp}}"
    AgentVersion = var.kmagent_version
  }
}

# --- GCP Source ---
source "googlecompute" "kmagent" {
  project_id          = var.gcp_project
  source_image_family = "ubuntu-2204-lts"
  zone                = var.gcp_zone
  machine_type        = "e2-micro"
  image_name          = "kloudmate-agent-{{timestamp}}"
  image_family        = "kloudmate-agent"
  ssh_username        = "ubuntu"
}

# --- AWS Source (Oracle Linux) ---
source "amazon-ebs" "kmagent-oracle" {
  ami_name      = "kloudmate-agent-ol-{{timestamp}}"
  instance_type = "t3.micro"
  region        = "us-east-1"

  source_ami_filter {
    filters = {
      name                = "Oracle-Linux-9.4-20*"
      root-device-type    = "ebs"
      virtualization-type = "hvm"
    }
    most_recent = true
    owners      = ["131827586825"] # Oracle
  }

  ssh_username = "ec2-user"

  tags = {
    Name        = "kloudmate-agent-oracle"
    BuildTime   = "{{timestamp}}"
    AgentVersion = var.kmagent_version
  }
}

# --- OCI Source ---
source "oracle-oci" "kmagent" {
  tenancy_ocid     = var.oci_tenancy_ocid
  user_ocid        = var.oci_user_ocid
  fingerprint      = var.oci_fingerprint
  key_file         = var.oci_key_file
  region           = var.oci_region
  compartment_ocid = var.oci_compartment_ocid
  subnet_ocid      = var.oci_subnet_ocid

  image_name   = "kloudmate-agent-{{timestamp}}"
  shape        = "VM.Standard.E4.Flex"
  ssh_username = "opc" # Oracle Linux standard user on OCI

  shape_config {
    ocpus         = 1
    memory_in_gbs = 4
  }

  source_image_filter {
    filters {
      operating_system         = "Oracle Linux"
      operating_system_version = "9"
    }
  }
}

# --- Build ---
build {
  sources = [
    "source.amazon-ebs.kmagent",
    "source.googlecompute.kmagent",
    "source.amazon-ebs.kmagent-oracle",
    "source.oracle-oci.kmagent",
  ]

  # Install kmagent (binary only - config injected at boot via user-data)
  provisioner "shell" {
    inline = [
      "if command -v apt-get &> /dev/null; then",
      "  sudo apt-get update -qq && sudo apt-get install -y -qq curl jq",
      "elif command -v dnf &> /dev/null; then",
      "  sudo dnf install -y -q curl jq",
      "elif command -v yum &> /dev/null; then",
      "  sudo yum install -y -q curl jq",
      "fi",
      "echo 'Installing kmagent ${var.kmagent_version}...'",
      "curl -fsSL https://raw.githubusercontent.com/kloudmate/km-agent/main/install.sh | sudo bash -s -- --install-only",
      "sudo systemctl enable kmagent",
      "echo 'kmagent installed, will be configured at boot via user-data'",
    ]
  }

  # Cleanup for smaller image
  provisioner "shell" {
    inline = [
      "if command -v apt-get &> /dev/null; then",
      "  sudo apt-get clean && sudo rm -rf /var/lib/apt/lists/*",
      "elif command -v dnf &> /dev/null; then",
      "  sudo dnf clean all",
      "elif command -v yum &> /dev/null; then",
      "  sudo yum clean all",
      "fi",
      "sudo rm -rf /tmp/* /var/tmp/*",
      "sudo truncate -s 0 /var/log/*.log",
    ]
  }
}
