package clouddetect

import (
	"context"
	"io"
	"net/http"
	"strings"
)

const (
	defaultAlibabaBaseURL = "http://100.100.100.200"
)

// alibabaDetector implements ProviderDetector for Alibaba Cloud.
type alibabaDetector struct {
	baseDetector
	logger detectorLogger
}

func newAlibabaDetector(client *http.Client, baseURL string, logger detectorLogger) *alibabaDetector {
	if baseURL == "" {
		baseURL = defaultAlibabaBaseURL
	}
	return &alibabaDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *alibabaDetector) Name() Provider { return ProviderAlibabaCloud }

func (d *alibabaDetector) Detect(ctx context.Context) *CloudEnvironment {
	// Alibaba Cloud ECS uses a different metadata IP: 100.100.100.200
	regionID, err := d.metadataGet(ctx, "latest/meta-data/region-id")
	if err != nil {
		d.logger.Debugw("Alibaba Cloud metadata request failed, not running on Alibaba", "error", err)
		return nil
	}

	instanceID, _ := d.metadataGet(ctx, "latest/meta-data/instance-id")
	zoneID, _ := d.metadataGet(ctx, "latest/meta-data/zone-id")
	instanceType, _ := d.metadataGet(ctx, "latest/meta-data/instance/instance-type")

	return &CloudEnvironment{
		Provider:    ProviderAlibabaCloud,
		ComputeType: ComputeAlibabaECS,
		Region:      regionID,
		InstanceID:  instanceID,
		Metadata: map[string]string{
			"zone_id":       zoneID,
			"instance_type": instanceType,
		},
	}
}

func (d *alibabaDetector) metadataGet(ctx context.Context, path string) (string, error) {
	url := d.baseURL + "/" + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", http.ErrNotSupported
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
