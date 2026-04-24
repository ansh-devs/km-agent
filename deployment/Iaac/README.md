# KloudMate Agent - Multi-Cloud Deployment Kit

Deploy the KloudMate agent (`kmagent`) across any cloud provider or on-premises infrastructure.

## Deployment Approaches

| Method | Best For | Cloud Support |
|--------|----------|---------------|
| **cloud-init** | New VMs, ASGs, Scale Sets | All providers |
| **Ansible** | Existing VM fleets | All providers + on-prem |
| **Terraform module** | IaC-managed infrastructure | All providers |
| **Packer** | Pre-baked machine images | AWS, GCP, Azure |

### Decision Matrix

```
New VM / Auto-scaling group?
  └─ Yes → cloud-init (via User Data / Custom Data)
  │         └─ Want faster boot? → Packer (pre-bake) + cloud-init (config only)
  └─ No → Existing fleet?
           └─ Yes → Ansible playbook
           └─ Managing infra with Terraform? → Terraform user-data module
```

---

## 1. cloud-init (Universal - All Cloud Providers)

Works on every provider that supports cloud-init (AWS, GCP, Azure, DigitalOcean, Alibaba, Hetzner, Vultr, Linode, etc.)

### AWS - Launch Template User Data
```bash
# Encode and attach to launch template
base64 -w0 cloud-init/cloud-init.yaml > /tmp/userdata.b64

aws ec2 create-launch-template \
  --launch-template-name kmagent-template \
  --launch-template-data '{
    "UserData": "'$(cat /tmp/userdata.b64)'"
  }'
```

### GCP - Instance Template
```bash
gcloud compute instance-templates create kmagent-template \
  --metadata-from-file user-data=cloud-init/cloud-init.yaml \
  --machine-type e2-medium \
  --image-family ubuntu-2204-lts \
  --image-project ubuntu-os-cloud
```

### Azure - VM Scale Set
```bash
az vmss create \
  --name kmagent-vmss \
  --resource-group myResourceGroup \
  --image Ubuntu2204 \
  --custom-data cloud-init/cloud-init.yaml
```

### DigitalOcean
```bash
doctl compute droplet create kmagent-node \
  --image ubuntu-22-04-x64 \
  --size s-1vcpu-1gb \
  --user-data-file cloud-init/cloud-init.yaml
```

### Alibaba Cloud
```bash
# Paste cloud-init.yaml content into:
# ECS Console → Instance → Advanced Options → User Data
```

---

## 2. Ansible (Existing VM Fleets)

### Quick Start

```bash
cd ansible/

# Set your API key
export KM_API_KEY="your-kloudmate-api-key"

# Edit inventory with your hosts
vim inventories/production.ini

# Deploy to all hosts
ansible-playbook -i inventories/production.ini deploy-kmagent.yml \
  -e "km_api_key=$KM_API_KEY km_endpoint=https://ingest.kloudmate.com"

# Deploy to specific cloud group only
ansible-playbook -i inventories/production.ini deploy-kmagent.yml \
  -e "km_api_key=$KM_API_KEY km_endpoint=https://ingest.kloudmate.com" \
  --limit aws

# Dry run
ansible-playbook -i inventories/production.ini deploy-kmagent.yml \
  -e "km_api_key=$KM_API_KEY km_endpoint=https://ingest.kloudmate.com" \
  --check --diff
```

### Dynamic Inventory (Auto-discover VMs)

Instead of static inventory files, use cloud-native dynamic inventory plugins:

```bash
# AWS
pip install boto3
ansible-playbook -i aws_ec2.yml deploy-kmagent.yml

# GCP
pip install google-auth
ansible-playbook -i gcp_compute.yml deploy-kmagent.yml

# Azure
pip install azure-identity azure-mgmt-compute
ansible-playbook -i azure_rm.yml deploy-kmagent.yml
```

### AWS EC2 Dynamic Inventory (`aws_ec2.yml`)
```yaml
plugin: amazon.aws.aws_ec2
regions:
  - us-east-1
  - us-west-2
  - ap-south-1
filters:
  tag:Environment: production
  instance-state-name: running
keyed_groups:
  - key: tags.Role
    prefix: role
  - key: placement.region
    prefix: region
compose:
  ansible_host: private_ip_address
```

### Useful Ansible Commands

```bash
# Upgrade agent on all hosts
ansible-playbook deploy-kmagent.yml --tags upgrade \
  -e "kmagent_version=1.3.0"

# Only reconfigure (no reinstall)
ansible-playbook deploy-kmagent.yml --tags configure

# Uninstall from specific hosts
ansible-playbook deploy-kmagent.yml \
  -e "kmagent_state=absent" --limit "gcp"

# Check agent status across fleet
ansible all -i inventories/production.ini -m shell \
  -a "systemctl status kmagent | head -5"
```

---

## 3. Terraform Module

### Usage with any provider

```hcl
module "kmagent" {
  source      = "./modules/user-data"
  km_api_key  = var.km_api_key
  km_endpoint = "https://ingest.kloudmate.com"
  kmagent_tags = {
    env  = "production"
    team = "platform"
  }
}

# AWS ASG
resource "aws_launch_template" "app" {
  name_prefix = "app-"
  user_data   = base64encode(module.kmagent.cloud_init)
}

resource "aws_autoscaling_group" "app" {
  launch_template {
    id      = aws_launch_template.app.id
    version = "$Latest"
  }
  min_size = 2
  max_size = 20
}

# GCP MIG
resource "google_compute_instance_template" "app" {
  metadata = {
    user-data = module.kmagent.cloud_init
  }
}

# Azure VMSS
resource "azurerm_linux_virtual_machine_scale_set" "app" {
  custom_data = base64encode(module.kmagent.cloud_init)
}

# DigitalOcean
resource "digitalocean_droplet" "app" {
  user_data = module.kmagent.cloud_init
}
```

---

## 4. Packer (Pre-baked Images)

Best when you want zero install-time latency. The agent binary is baked into the image; only the config (API key) is injected at boot.

```bash
cd packer/

# Build AWS AMI
packer build -only=amazon-ebs.kmagent \
  -var 'kmagent_version=1.2.0' \
  kmagent-image.pkr.hcl

# Build GCP image
packer build -only=googlecompute.kmagent \
  -var 'gcp_project=my-project' \
  kmagent-image.pkr.hcl

# Build all
packer build kmagent-image.pkr.hcl
```

Then use the resulting image in your Launch Template / Instance Template, with minimal user-data that only injects the API key.

---

## Secret Management

Never hardcode API keys. Use your cloud's native secret store:

| Provider | Service | Retrieval |
|----------|---------|-----------|
| AWS | SSM Parameter Store | `aws ssm get-parameter --name /kloudmate/api-key --with-decryption` |
| AWS | Secrets Manager | `aws secretsmanager get-secret-value --secret-id kloudmate-api-key` |
| GCP | Secret Manager | `gcloud secrets versions access latest --secret=kloudmate-api-key` |
| Azure | Key Vault | `az keyvault secret show --name kloudmate-api-key --vault-name myvault` |
| DO | Reserved env vars | Set via doctl or Terraform |
| Alibaba | KMS | `aliyun kms GetSecretValue --SecretName kloudmate-api-key` |

---

## Directory Structure

```
kmagent-deploy/
├── README.md
├── ansible/
│   ├── ansible.cfg
│   ├── deploy-kmagent.yml          # Main playbook
│   ├── inventories/
│   │   └── production.ini          # Static inventory (edit with your hosts)
│   └── roles/
│       └── kmagent/
│           ├── defaults/main.yml   # Configurable variables
│           ├── handlers/main.yml   # Service reload/restart
│           ├── tasks/main.yml      # Install, configure, upgrade, uninstall
│           └── templates/
│               ├── agent.yaml.j2   # Agent config
│               └── kmagent.service.j2  # systemd unit
├── cloud-init/
│   └── cloud-init.yaml             # Universal cloud-init config
├── packer/
│   └── kmagent-image.pkr.hcl       # Multi-cloud image builder
└── terraform/
    └── modules/
        └── user-data/
            └── main.tf              # Reusable user-data module
```
