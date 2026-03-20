package clouddetect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const (
	defaultAzureIMDSBaseURL = "http://169.254.169.254"
)

// azureDetector implements ProviderDetector for Microsoft Azure.
type azureDetector struct {
	baseDetector
	logger detectorLogger
}

// newAzureDetector creates a new Azure provider detector.
func newAzureDetector(client *http.Client, baseURL string, logger detectorLogger) *azureDetector {
	if baseURL == "" {
		baseURL = defaultAzureIMDSBaseURL
	}
	return &azureDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *azureDetector) Name() Provider { return ProviderAzure }

// azureIMDSResponse represents the Azure IMDS response.
type azureIMDSResponse struct {
	Compute struct {
		Location          string `json:"location"`
		Name              string `json:"name"`
		ResourceGroupName string `json:"resourceGroupName"`
		SubscriptionID    string `json:"subscriptionId"`
		VMID              string `json:"vmId"`
		VMSize            string `json:"vmSize"`
	} `json:"compute"`
	Network struct {
		Interface []struct {
			IPv4 struct {
				Subnet []struct {
					Address string `json:"address"`
					Prefix  string `json:"prefix"`
				} `json:"subnet"`
			} `json:"ipv4"`
		} `json:"interface"`
	} `json:"network"`
}

func (d *azureDetector) Detect(ctx context.Context) *CloudEnvironment {
	url := d.baseURL + "/metadata/instance?api-version=2021-02-01"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Metadata", "true")

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Debugw("Azure IMDS request failed, not running on Azure", "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var azureMeta azureIMDSResponse
	if err := json.Unmarshal(body, &azureMeta); err != nil {
		d.logger.Debugw("failed to parse Azure IMDS response", "error", err)
		return nil
	}

	env := &CloudEnvironment{
		Provider:   ProviderAzure,
		Region:     azureMeta.Compute.Location,
		AccountID:  azureMeta.Compute.SubscriptionID,
		InstanceID: azureMeta.Compute.Name,
		Metadata: map[string]string{
			"resource_group": azureMeta.Compute.ResourceGroupName,
			"vm_id":          azureMeta.Compute.VMID,
			"vm_size":        azureMeta.Compute.VMSize,
		},
	}

	// Determine compute type
	if isKubernetes() {
		env.ComputeType = ComputeAKS
	} else {
		env.ComputeType = ComputeAzureVM
	}

	return env
}
