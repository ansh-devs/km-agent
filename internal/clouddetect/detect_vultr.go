package clouddetect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const (
	defaultVultrBaseURL = "http://169.254.169.254"
)

// vultrDetector implements ProviderDetector for Vultr.
type vultrDetector struct {
	baseDetector
	logger detectorLogger
}

func newVultrDetector(client *http.Client, baseURL string, logger detectorLogger) *vultrDetector {
	if baseURL == "" {
		baseURL = defaultVultrBaseURL
	}
	return &vultrDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *vultrDetector) Name() Provider { return ProviderVultr }

type vultrMetadata struct {
	InstanceID string `json:"instanceid"`
	Hostname   string `json:"hostname"`
	Region     struct {
		RegionCode string `json:"regioncode"`
	} `json:"region"`
}

func (d *vultrDetector) Detect(ctx context.Context) *CloudEnvironment {
	url := d.baseURL + "/v1.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Debugw("Vultr metadata request failed, not running on Vultr", "error", err)
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

	var meta vultrMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		d.logger.Debugw("failed to parse Vultr metadata", "error", err)
		return nil
	}

	return &CloudEnvironment{
		Provider:    ProviderVultr,
		ComputeType: ComputeVultr,
		Region:      meta.Region.RegionCode,
		InstanceID:  meta.InstanceID,
		Metadata: map[string]string{
			"hostname": meta.Hostname,
		},
	}
}
