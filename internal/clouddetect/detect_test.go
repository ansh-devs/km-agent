package clouddetect

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.uber.org/zap"
)

func testLogger() *zap.SugaredLogger {
	l, _ := zap.NewDevelopment()
	return l.Sugar()
}

// ---------- AWS Tests ----------

func TestDetectAWS_EC2(t *testing.T) {
	identity := awsIdentityDocument{
		AccountID:        "123456789012",
		Region:           "us-east-1",
		AvailabilityZone: "us-east-1a",
		InstanceID:       "i-0abcdef1234567890",
		InstanceType:     "m5.large",
		ImageID:          "ami-0abcdef1234567890",
	}
	identityJSON, _ := json.Marshal(identity)

	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Write([]byte("test-token"))
	})
	mux.HandleFunc("/latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-aws-ec2-metadata-token") != "test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write(identityJSON)
	})
	mux.HandleFunc("/latest/meta-data/mac", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0a:1b:2c:3d:4e:5f"))
	})
	mux.HandleFunc("/latest/meta-data/network/interfaces/macs/0a:1b:2c:3d:4e:5f/vpc-id", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("vpc-0abcdef1234567890"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	aws := newAWSDetector(server.Client(), server.URL, testLogger())
	env := aws.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderAWS {
		t.Errorf("expected provider AWS, got %s", env.Provider)
	}
	if env.ComputeType != ComputeEC2 {
		t.Errorf("expected compute type ec2, got %s", env.ComputeType)
	}
	if env.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", env.Region)
	}
	if env.VpcID != "vpc-0abcdef1234567890" {
		t.Errorf("expected vpc vpc-0abcdef1234567890, got %s", env.VpcID)
	}
}

func TestDetectAWS_EKS(t *testing.T) {
	identity := awsIdentityDocument{
		AccountID:  "123456789012",
		Region:     "us-west-2",
		InstanceID: "i-eks123",
	}
	identityJSON, _ := json.Marshal(identity)

	mux := http.NewServeMux()
	mux.HandleFunc("/latest/api/token", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("t")) })
	mux.HandleFunc("/latest/dynamic/instance-identity/document", func(w http.ResponseWriter, r *http.Request) { w.Write(identityJSON) })
	mux.HandleFunc("/latest/meta-data/mac", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("aa:bb:cc:dd:ee:ff")) })
	mux.HandleFunc("/latest/meta-data/network/interfaces/macs/aa:bb:cc:dd:ee:ff/vpc-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("vpc-eks")) })

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	aws := newAWSDetector(server.Client(), server.URL, testLogger())
	env := aws.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeEKS {
		t.Errorf("expected compute type eks, got %s", env.ComputeType)
	}
}

func TestDetectAWS_Fargate(t *testing.T) {
	taskMux := http.NewServeMux()
	taskMux.HandleFunc("/task", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"LaunchType": "FARGATE",
			"TaskARN":    "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123",
		})
	})
	taskServer := httptest.NewServer(taskMux)
	defer taskServer.Close()

	t.Setenv(ecsMetadataURIV4, taskServer.URL)
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	aws := newAWSDetector(taskServer.Client(), "http://192.0.2.1", testLogger())
	env := aws.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeFargate {
		t.Errorf("expected compute type fargate, got %s", env.ComputeType)
	}
	if env.Region != "us-east-1" {
		t.Errorf("expected region us-east-1, got %s", env.Region)
	}
}

func TestDetectAWS_Lambda(t *testing.T) {
	t.Setenv("AWS_LAMBDA_FUNCTION_NAME", "my-function")
	t.Setenv("AWS_LAMBDA_FUNCTION_VERSION", "$LATEST")
	t.Setenv("AWS_REGION", "eu-west-1")
	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)

	aws := newAWSDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger())
	env := aws.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeLambda {
		t.Errorf("expected compute type lambda, got %s", env.ComputeType)
	}
	if env.Region != "eu-west-1" {
		t.Errorf("expected region eu-west-1, got %s", env.Region)
	}
	if env.Metadata["function_name"] != "my-function" {
		t.Errorf("expected function_name my-function, got %s", env.Metadata["function_name"])
	}
}

// ---------- GCP Tests ----------

func TestDetectGCP_GCE(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/project/project-id", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Write([]byte("my-gcp-project"))
	})
	mux.HandleFunc("/instance/zone", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("projects/123/zones/us-central1-a")) })
	mux.HandleFunc("/instance/name", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("my-instance")) })
	mux.HandleFunc("/instance/id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("1234567890")) })
	mux.HandleFunc("/instance/attributes/cluster-name", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/instance/network-interfaces/0/network", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("projects/123/global/networks/default")) })

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("K_SERVICE")
	os.Unsetenv("FUNCTION_TARGET")
	os.Unsetenv("FUNCTION_NAME")
	os.Unsetenv("GAE_SERVICE")

	gcp := newGCPDetector(server.Client(), server.URL+"/", testLogger())
	env := gcp.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeGCE {
		t.Errorf("expected compute type gce, got %s", env.ComputeType)
	}
	if env.Region != "us-central1" {
		t.Errorf("expected region us-central1, got %s", env.Region)
	}
}

func TestDetectGCP_GKE(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/project/project-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("my-gke-project")) })
	mux.HandleFunc("/instance/zone", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("projects/123/zones/europe-west1-b")) })
	mux.HandleFunc("/instance/name", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("gke-node-1")) })
	mux.HandleFunc("/instance/id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("9876543210")) })
	mux.HandleFunc("/instance/attributes/cluster-name", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("my-cluster")) })
	mux.HandleFunc("/instance/network-interfaces/0/network", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("gke-vpc")) })

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv("K_SERVICE")
	os.Unsetenv("FUNCTION_TARGET")
	os.Unsetenv("FUNCTION_NAME")
	os.Unsetenv("GAE_SERVICE")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	gcp := newGCPDetector(server.Client(), server.URL+"/", testLogger())
	env := gcp.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeGKE {
		t.Errorf("expected compute type gke, got %s", env.ComputeType)
	}
}

func TestDetectGCP_CloudRun(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/project/project-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("my-project")) })
	mux.HandleFunc("/instance/zone", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("projects/1/zones/us-central1-1")) })
	mux.HandleFunc("/instance/name", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("")) })
	mux.HandleFunc("/instance/id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("")) })
	mux.HandleFunc("/instance/network-interfaces/0/network", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv("FUNCTION_TARGET")
	os.Unsetenv("GAE_SERVICE")
	t.Setenv("K_SERVICE", "my-cloud-run-svc")

	gcp := newGCPDetector(server.Client(), server.URL+"/", testLogger())
	env := gcp.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeCloudRun {
		t.Errorf("expected compute type cloud_run, got %s", env.ComputeType)
	}
}

// ---------- Azure Tests ----------

func TestDetectAzure_VM(t *testing.T) {
	azureResp := azureIMDSResponse{}
	azureResp.Compute.Location = "eastus"
	azureResp.Compute.Name = "my-vm"
	azureResp.Compute.SubscriptionID = "sub-123"
	azureResp.Compute.VMID = "vm-456"
	azureResp.Compute.VMSize = "Standard_D2s_v3"
	azureResp.Compute.ResourceGroupName = "my-rg"
	azureJSON, _ := json.Marshal(azureResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/instance", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata") != "true" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Write(azureJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	os.Unsetenv("KUBERNETES_SERVICE_HOST")

	azure := newAzureDetector(server.Client(), server.URL, testLogger())
	env := azure.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderAzure {
		t.Errorf("expected provider Azure, got %s", env.Provider)
	}
	if env.ComputeType != ComputeAzureVM {
		t.Errorf("expected compute type azure_vm, got %s", env.ComputeType)
	}
}

func TestDetectAzure_AKS(t *testing.T) {
	azureResp := azureIMDSResponse{}
	azureResp.Compute.Location = "westeurope"
	azureResp.Compute.Name = "aks-node"
	azureResp.Compute.SubscriptionID = "sub-789"
	azureJSON, _ := json.Marshal(azureResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/instance", func(w http.ResponseWriter, r *http.Request) { w.Write(azureJSON) })

	server := httptest.NewServer(mux)
	defer server.Close()

	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")

	azure := newAzureDetector(server.Client(), server.URL, testLogger())
	env := azure.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.ComputeType != ComputeAKS {
		t.Errorf("expected compute type aks, got %s", env.ComputeType)
	}
}

// ---------- OCI Test ----------

func TestDetectOCI(t *testing.T) {
	ociResp := ociInstanceMetadata{
		DisplayName:         "my-oci-instance",
		ID:                  "ocid1.instance.oc1.iad.abc123",
		CanonicalRegionName: "us-ashburn-1",
		AvailabilityDomain:  "AD-1",
		FaultDomain:         "FD-1",
		Shape:               "VM.Standard.E4.Flex",
		CompartmentID:       "ocid1.compartment.oc1..xyz789",
	}
	ociJSON, _ := json.Marshal(ociResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/opc/v2/instance/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(ociJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	oci := newOCIDetector(server.Client(), server.URL, testLogger())
	env := oci.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderOCI {
		t.Errorf("expected provider oci, got %s", env.Provider)
	}
	if env.Region != "us-ashburn-1" {
		t.Errorf("expected region us-ashburn-1, got %s", env.Region)
	}
	if env.ComputeType != ComputeOCI {
		t.Errorf("expected compute type oci, got %s", env.ComputeType)
	}
}

// ---------- DigitalOcean Test ----------

func TestDetectDigitalOcean(t *testing.T) {
	doResp := doMetadata{
		DropletID: 12345,
		Hostname:  "my-droplet",
		Region:    "nyc1",
	}
	doJSON, _ := json.Marshal(doResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/metadata/v1.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write(doJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	do := newDigitalOceanDetector(server.Client(), server.URL, testLogger())
	env := do.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderDigitalOcean {
		t.Errorf("expected provider digitalocean, got %s", env.Provider)
	}
	if env.Region != "nyc1" {
		t.Errorf("expected region nyc1, got %s", env.Region)
	}
}

func TestDetectVultr(t *testing.T) {
	vultrResp := vultrMetadata{
		InstanceID: "v-abc123",
		Hostname:   "my-vultr",
	}
	vultrResp.Region.RegionCode = "ewr"
	vultrJSON, _ := json.Marshal(vultrResp)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write(vultrJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	v := newVultrDetector(server.Client(), server.URL, testLogger())
	env := v.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderVultr {
		t.Errorf("expected provider vultr, got %s", env.Provider)
	}
	if env.Region != "ewr" {
		t.Errorf("expected region ewr, got %s", env.Region)
	}
}

func TestDetectAlibaba(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/latest/meta-data/region-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("cn-hangzhou")) })
	mux.HandleFunc("/latest/meta-data/instance-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("i-abc123")) })
	mux.HandleFunc("/latest/meta-data/zone-id", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("cn-hangzhou-b")) })
	mux.HandleFunc("/latest/meta-data/instance/instance-type", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ecs.g6.large")) })

	server := httptest.NewServer(mux)
	defer server.Close()

	ali := newAlibabaDetector(server.Client(), server.URL, testLogger())
	env := ali.Detect(context.Background())

	if env == nil {
		t.Fatal("expected non-nil environment")
	}
	if env.Provider != ProviderAlibabaCloud {
		t.Errorf("expected provider alibaba_cloud, got %s", env.Provider)
	}
	if env.Region != "cn-hangzhou" {
		t.Errorf("expected region cn-hangzhou, got %s", env.Region)
	}
}

func TestDetectOnPrem_BareMetal(t *testing.T) {
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	d := NewDetector(testLogger())
	d.providers = []ProviderDetector{
		newAWSDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger()),
		newGCPDetector(&http.Client{Timeout: 1}, "http://192.0.2.1/", testLogger()),
		newAzureDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger()),
	}

	env := d.Detect(context.Background())

	if env.Provider != ProviderOnPrem {
		t.Errorf("expected provider on-prem, got %s", env.Provider)
	}
	if env.ComputeType != ComputeBareMetal {
		t.Errorf("expected compute type bare_metal, got %s", env.ComputeType)
	}
	if env.Hostname == "" {
		t.Error("expected hostname to be populated")
	}
	if env.OS == "" {
		t.Error("expected OS to be populated")
	}
}

func TestDetectOnPrem_Kubernetes(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	d := NewDetector(testLogger())
	d.providers = []ProviderDetector{
		newAWSDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger()),
		newGCPDetector(&http.Client{Timeout: 1}, "http://192.0.2.1/", testLogger()),
		newAzureDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger()),
	}

	env := d.Detect(context.Background())

	if env.Provider != ProviderOnPrem {
		t.Errorf("expected provider on-prem, got %s", env.Provider)
	}
	if env.ComputeType != ComputeKubernetes {
		t.Errorf("expected compute type kubernetes, got %s", env.ComputeType)
	}
}

func TestRecommendedDetectors_EC2(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderAWS, ComputeType: ComputeEC2, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "system", "ec2")
	assertNotContains(t, detectors, "eks", "ecs", "lambda")
}

func TestRecommendedDetectors_EKS(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderAWS, ComputeType: ComputeEKS, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "system", "ec2", "eks", "k8snode")
}

func TestRecommendedDetectors_Lambda(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderAWS, ComputeType: ComputeLambda, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "lambda")
	assertNotContains(t, detectors, "ec2", "ecs")
}

func TestRecommendedDetectors_GKE(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderGCP, ComputeType: ComputeGKE, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "system", "gcp", "k8snode")
}

func TestRecommendedDetectors_AKS(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderAzure, ComputeType: ComputeAKS, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "system", "azure", "aks", "k8snode")
}

func TestRecommendedDetectors_OnPrem_K8s(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderOnPrem, ComputeType: ComputeKubernetes, Metadata: map[string]string{}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "env", "system", "k8snode", "kubeadm")
}

func TestRecommendedDetectors_Docker(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderOnPrem, ComputeType: ComputeBareMetal, Metadata: map[string]string{"docker": "true"}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "docker")
}

func TestRecommendedDetectors_Consul(t *testing.T) {
	env := &CloudEnvironment{Provider: ProviderAWS, ComputeType: ComputeEC2, Metadata: map[string]string{"consul": "true"}}
	detectors := env.RecommendedDetectors()
	assertContains(t, detectors, "ec2", "consul")
}

func TestDetectCaching(t *testing.T) {
	os.Unsetenv("KUBERNETES_SERVICE_HOST")
	os.Unsetenv(ecsMetadataURIV4)
	os.Unsetenv(ecsMetadataURIV3)
	os.Unsetenv("AWS_LAMBDA_FUNCTION_NAME")

	d := NewDetector(testLogger())
	d.providers = []ProviderDetector{
		newAWSDetector(&http.Client{Timeout: 1}, "http://192.0.2.1", testLogger()),
	}

	env1 := d.Detect(context.Background())
	env2 := d.Detect(context.Background())

	if env1 != env2 {
		t.Error("expected same pointer from cached detection")
	}

	d.Reset()
	env3 := d.Detect(context.Background())
	if env1 == env3 {
		t.Error("expected different pointer after reset")
	}
}

func assertContains(t *testing.T, slice []string, items ...string) {
	t.Helper()
	for _, item := range items {
		found := false
		for _, s := range slice {
			if s == item {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %v to contain %q", slice, item)
		}
	}
}

func assertNotContains(t *testing.T, slice []string, items ...string) {
	t.Helper()
	for _, item := range items {
		for _, s := range slice {
			if s == item {
				t.Errorf("expected %v to NOT contain %q", slice, item)
			}
		}
	}
}
