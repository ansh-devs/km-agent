package clouddetect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const (
	defaultOCIIMDSBaseURL = "http://169.254.169.254"
)

// ociDetector implements ProviderDetector for Oracle Cloud Infrastructure.
type ociDetector struct {
	baseDetector
	logger detectorLogger
}

func newOCIDetector(client *http.Client, baseURL string, logger detectorLogger) *ociDetector {
	if baseURL == "" {
		baseURL = defaultOCIIMDSBaseURL
	}
	return &ociDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *ociDetector) Name() Provider { return ProviderOCI }

// ociInstanceMetadata represents OCI instance metadata.
type ociInstanceMetadata struct {
	DisplayName         string `json:"displayName"`
	ID                  string `json:"id"`
	Region              string `json:"region"`
	CanonicalRegionName string `json:"canonicalRegionName"`
	AvailabilityDomain  string `json:"availabilityDomain"`
	FaultDomain         string `json:"faultDomain"`
	Shape               string `json:"shape"`
	CompartmentID       string `json:"compartmentId"`
}

func (d *ociDetector) Detect(ctx context.Context) *CloudEnvironment {
	url := d.baseURL + "/opc/v2/instance/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer Oracle")

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Debugw("OCI IMDS request failed, not running on OCI", "error", err)
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

	var meta ociInstanceMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		d.logger.Debugw("failed to parse OCI IMDS response", "error", err)
		return nil
	}

	return &CloudEnvironment{
		Provider:    ProviderOCI,
		ComputeType: ComputeOCI,
		Region:      meta.CanonicalRegionName,
		AccountID:   meta.CompartmentID,
		InstanceID:  meta.ID,
		Metadata: map[string]string{
			"display_name":        meta.DisplayName,
			"shape":               meta.Shape,
			"availability_domain": meta.AvailabilityDomain,
			"fault_domain":        meta.FaultDomain,
		},
	}
}
