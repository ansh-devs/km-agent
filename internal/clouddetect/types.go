package clouddetect

// Provider represents a cloud infrastructure provider.
type Provider string

const (
	ProviderAWS          Provider = "aws"
	ProviderGCP          Provider = "gcp"
	ProviderAzure        Provider = "azure"
	ProviderOCI          Provider = "oci"
	ProviderDigitalOcean Provider = "digitalocean"
	ProviderVultr        Provider = "vultr"
	ProviderAlibabaCloud Provider = "alibaba_cloud"
	ProviderOnPrem       Provider = "on-prem"
)

// ComputeType represents the specific compute platform within a cloud provider.
type ComputeType string

const (
	// AWS compute types
	ComputeEC2              ComputeType = "ec2"
	ComputeECS              ComputeType = "ecs"     // ECS on EC2
	ComputeFargate          ComputeType = "fargate" // ECS on Fargate
	ComputeEKS              ComputeType = "eks"
	ComputeLambda           ComputeType = "lambda"
	ComputeElasticBeanstalk ComputeType = "elastic_beanstalk"

	// GCP compute types
	ComputeGCE            ComputeType = "gce"
	ComputeGKE            ComputeType = "gke"
	ComputeCloudRun       ComputeType = "cloud_run"
	ComputeCloudFunctions ComputeType = "cloud_functions"
	ComputeAppEngine      ComputeType = "app_engine"

	// Azure compute types
	ComputeAzureVM ComputeType = "azure_vm"
	ComputeAKS     ComputeType = "aks"

	// Other cloud compute types
	ComputeOCI          ComputeType = "oci"          // Oracle Cloud Infrastructure
	ComputeAlibabaECS   ComputeType = "alibaba_ecs"  // Alibaba Cloud ECS
	ComputeDigitalOcean ComputeType = "digitalocean" // DigitalOcean Droplet
	ComputeVultr        ComputeType = "vultr"        // Vultr instance

	// Generic / cloud-agnostic compute types
	ComputeVM         ComputeType = "vm"
	ComputeKubernetes ComputeType = "kubernetes"
	ComputeBareMetal  ComputeType = "bare_metal"

	ComputeUnknown ComputeType = "unknown"
)

// CloudEnvironment holds the detected cloud environment information.
type CloudEnvironment struct {
	Provider    Provider
	ComputeType ComputeType
	Region      string
	AccountID   string // AWS account ID, GCP project ID, Azure subscription ID, etc.
	InstanceID  string // EC2 instance ID, GCE instance name, Azure VM name, etc.
	VpcID       string // VPC/VNet ID
	Hostname    string // OS hostname
	OS          string // Runtime OS: "linux", "windows", "darwin"
	Metadata    map[string]string
}

// RecommendedDetectors returns the list of OTel resourcedetectionprocessor
// detector names that should be enabled for this detected environment.
func (e *CloudEnvironment) RecommendedDetectors() []string {
	// Always include env + system
	detectors := []string{"env", "system"}

	switch e.Provider {
	case ProviderAWS:
		switch e.ComputeType {
		case ComputeEC2:
			detectors = append(detectors, "ec2")
		case ComputeEKS:
			detectors = append(detectors, "ec2", "eks", "k8snode")
		case ComputeECS, ComputeFargate:
			detectors = append(detectors, "ecs")
		case ComputeLambda:
			detectors = append(detectors, "lambda")
		case ComputeElasticBeanstalk:
			detectors = append(detectors, "ec2", "elastic_beanstalk")
		}

	case ProviderGCP:
		detectors = append(detectors, "gcp")
		switch e.ComputeType {
		case ComputeGKE:
			detectors = append(detectors, "k8snode")
		}

	case ProviderAzure:
		detectors = append(detectors, "azure")
		switch e.ComputeType {
		case ComputeAKS:
			detectors = append(detectors, "aks", "k8snode")
		}

	case ProviderOCI:
		detectors = append(detectors, "oraclecloud")

	case ProviderDigitalOcean:
		detectors = append(detectors, "digitalocean")

	case ProviderVultr:
		detectors = append(detectors, "vultr")

	case ProviderAlibabaCloud:
		detectors = append(detectors, "alibaba_ecs")

	case ProviderOnPrem:
		if e.ComputeType == ComputeKubernetes {
			detectors = append(detectors, "k8snode", "kubeadm")
		}
	}

	// Cross-cutting detectors based on signals, not provider
	if e.Metadata != nil {
		if e.Metadata["docker"] == "true" {
			detectors = append(detectors, "docker")
		}
		if e.Metadata["consul"] == "true" {
			detectors = append(detectors, "consul")
		}
		if e.Metadata["openshift"] == "true" {
			detectors = append(detectors, "openshift")
		}
	}

	return detectors
}
