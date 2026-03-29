package clouddetect

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const (
	defaultDigitalOceanBaseURL = "http://169.254.169.254"
)

// digitalOceanDetector implements ProviderDetector for DigitalOcean.
type digitalOceanDetector struct {
	baseDetector
	logger detectorLogger
}

func newDigitalOceanDetector(client *http.Client, baseURL string, logger detectorLogger) *digitalOceanDetector {
	if baseURL == "" {
		baseURL = defaultDigitalOceanBaseURL
	}
	return &digitalOceanDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *digitalOceanDetector) Name() Provider { return ProviderDigitalOcean }

type doMetadata struct {
	DropletID  int    `json:"droplet_id"`
	Hostname   string `json:"hostname"`
	Region     string `json:"region"`
	Interfaces struct {
		Private []struct {
			IPv4 struct {
				IPAddress string `json:"ip_address"`
			} `json:"ipv4"`
		} `json:"private"`
	} `json:"interfaces"`
}

func (d *digitalOceanDetector) Detect(ctx context.Context) *CloudEnvironment {
	url := d.baseURL + "/metadata/v1.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}

	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.Debugw("DigitalOcean metadata request failed, not running on DO", "error", err)
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

	var meta doMetadata
	if err := json.Unmarshal(body, &meta); err != nil {
		d.logger.Debugw("failed to parse DigitalOcean metadata", "error", err)
		return nil
	}

	return &CloudEnvironment{
		Provider:    ProviderDigitalOcean,
		ComputeType: ComputeDigitalOcean,
		Region:      meta.Region,
		InstanceID:  meta.Hostname,
		Metadata: map[string]string{
			"hostname": meta.Hostname,
		},
	}
}
