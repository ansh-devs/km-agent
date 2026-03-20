package clouddetect

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	defaultGCPMetadataBaseURL = "http://metadata.google.internal/computeMetadata/v1/"
)

// gcpDetector implements ProviderDetector for Google Cloud Platform.
type gcpDetector struct {
	baseDetector
	logger detectorLogger
}

func newGCPDetector(client *http.Client, baseURL string, logger detectorLogger) *gcpDetector {
	if baseURL == "" {
		baseURL = defaultGCPMetadataBaseURL
	}
	return &gcpDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *gcpDetector) Name() Provider { return ProviderGCP }

func (d *gcpDetector) Detect(ctx context.Context) *CloudEnvironment {
	projectID, err := d.metadataGet(ctx, "project/project-id")
	if err != nil {
		d.logger.Debugw("GCP metadata request failed, not running on GCP", "error", err)
		return nil
	}

	zone, _ := d.metadataGet(ctx, "instance/zone")
	instanceName, _ := d.metadataGet(ctx, "instance/name")
	instanceID, _ := d.metadataGet(ctx, "instance/id")

	// Extract region from zone (e.g., "projects/lunar-parsec/zones/us-central1-a" → "us-central1")
	region := ""
	if zone != "" {
		parts := strings.Split(zone, "/")
		zoneName := parts[len(parts)-1]
		lastDash := strings.LastIndex(zoneName, "-")
		if lastDash > 0 {
			region = zoneName[:lastDash]
		}
	}

	env := &CloudEnvironment{
		Provider:   ProviderGCP,
		Region:     region,
		AccountID:  projectID,
		InstanceID: instanceName,
		Metadata: map[string]string{
			"instance_id_numeric": instanceID,
			"zone":                zone,
		},
	}
	// Classify compute type
	env.ComputeType = d.classifyComputeType(ctx)
	// Network info
	networkInterfaces, _ := d.metadataGet(ctx, "instance/network-interfaces/0/network")
	if networkInterfaces != "" {
		env.VpcID = networkInterfaces
	}
	return env
}

func (d *gcpDetector) classifyComputeType(ctx context.Context) ComputeType {
	// Cloud Run Services/Jobs: K_SERVICE or K_REVISION env vars
	if os.Getenv("K_SERVICE") != "" || os.Getenv("K_REVISION") != "" {
		return ComputeCloudRun
	}
	// Cloud Functions: FUNCTION_TARGET (Gen2) or FUNCTION_NAME (Gen1)
	if os.Getenv("FUNCTION_TARGET") != "" || os.Getenv("FUNCTION_NAME") != "" {
		return ComputeCloudFunctions
	}
	// App Engine: GAE_SERVICE env var
	if os.Getenv("GAE_SERVICE") != "" {
		return ComputeAppEngine
	}
	// GKE: Kubernetes signals + GCP metadata
	if isKubernetes() {
		clusterName, _ := d.metadataGet(ctx, "instance/attributes/cluster-name")
		if clusterName != "" {
			return ComputeGKE
		}
		return ComputeGKE // Kubernetes on GCP = GKE
	}
	return ComputeGCE
}

func (d *gcpDetector) metadataGet(ctx context.Context, path string) (string, error) {
	base := d.baseURL
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	url := base + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GCP metadata request to %s returned status %d", path, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}
