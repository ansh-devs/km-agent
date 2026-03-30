package clouddetect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	defaultAWSIMDSBaseURL = "http://169.254.169.254"
	ecsMetadataURIV4      = "ECS_CONTAINER_METADATA_URI_V4"
	ecsMetadataURIV3      = "ECS_CONTAINER_METADATA_URI"
)

// awsDetector implements ProviderDetector for Amazon Web Services.
type awsDetector struct {
	baseDetector
	logger detectorLogger
}

func newAWSDetector(client *http.Client, baseURL string, logger detectorLogger) *awsDetector {
	if baseURL == "" {
		baseURL = defaultAWSIMDSBaseURL
	}
	return &awsDetector{
		baseDetector: baseDetector{client: client, baseURL: baseURL},
		logger:       logger,
	}
}

func (d *awsDetector) Name() Provider { return ProviderAWS }

// awsIdentityDocument represents the EC2 instance identity document.
// reference: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/instance-identity-documents.html
type awsIdentityDocument struct {
	AccountID        string `json:"accountId"`
	Region           string `json:"region"`
	AvailabilityZone string `json:"availabilityZone"`
	InstanceID       string `json:"instanceId"`
	InstanceType     string `json:"instanceType"`
	ImageID          string `json:"imageId"`
}

func (d *awsDetector) Detect(ctx context.Context) *CloudEnvironment {
	// Check for AWS Lambda first (env-based, no IMDS needed)
	if fn := os.Getenv("AWS_LAMBDA_FUNCTION_NAME"); fn != "" {
		return d.detectLambda()
	}

	tokenURL := d.baseURL + "/latest/api/token"
	dynamicURL := d.baseURL + "/latest/dynamic/instance-identity/document"
	metadataURL := d.baseURL + "/latest/meta-data/"

	// Try to get an IMDSv2 token
	token, err := d.getIMDSToken(ctx, tokenURL)
	if err != nil {
		d.logger.Debugw("AWS IMDS v2 token request failed, not running on AWS EC2", "error", err)
		return d.detectECSOnly(ctx)
	}

	// Getting the instance identity document
	identityBody, err := d.httpGetWithHeader(ctx, dynamicURL, "X-aws-ec2-metadata-token", token)
	if err != nil {
		d.logger.Debugw("AWS instance identity document request failed", "error", err)
		return nil
	}

	var identity awsIdentityDocument
	if err := json.Unmarshal(identityBody, &identity); err != nil {
		d.logger.Debugw("failed to parse AWS identity document", "error", err)
		return nil
	}

	// Try to get VPC ID
	vpcID := d.getVpcID(ctx, metadataURL, token)

	env := &CloudEnvironment{
		Provider:   ProviderAWS,
		Region:     identity.Region,
		AccountID:  identity.AccountID,
		InstanceID: identity.InstanceID,
		VpcID:      vpcID,
		Metadata: map[string]string{
			"availability_zone": identity.AvailabilityZone,
			"instance_type":     identity.InstanceType,
			"image_id":          identity.ImageID,
		},
	}

	//  compute type
	env.ComputeType = d.classifyComputeType(ctx)

	return env
}

func (d *awsDetector) detectLambda() *CloudEnvironment {
	return &CloudEnvironment{
		Provider:    ProviderAWS,
		ComputeType: ComputeLambda,
		Region:      os.Getenv("AWS_REGION"),
		Metadata: map[string]string{
			"function_name":    os.Getenv("AWS_LAMBDA_FUNCTION_NAME"),
			"function_version": os.Getenv("AWS_LAMBDA_FUNCTION_VERSION"),
		},
	}
}

func (d *awsDetector) classifyComputeType(ctx context.Context) ComputeType {
	// Check for Elastic Beanstalk
	if _, err := os.Stat("/var/elasticbeanstalk/xray/environment.conf"); err == nil {
		return ComputeElasticBeanstalk
	}
	// Check for ECS/Fargate
	if ecsType := d.detectECSComputeType(ctx); ecsType != ComputeUnknown {
		return ecsType
	}
	// Check for EKS
	if isKubernetes() {
		return ComputeEKS
	}
	return ComputeEC2
}

func (d *awsDetector) detectECSComputeType(ctx context.Context) ComputeType {
	metadataURI := os.Getenv(ecsMetadataURIV4)
	if metadataURI == "" {
		metadataURI = os.Getenv(ecsMetadataURIV3)
	}
	if metadataURI == "" {
		return ComputeUnknown
	}

	taskURL := metadataURI + "/task"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
	if err != nil {
		return ComputeECS
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return ComputeECS
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ComputeECS
	}

	var taskMeta struct {
		LaunchType string `json:"LaunchType"`
	}
	if err := json.Unmarshal(body, &taskMeta); err != nil {
		return ComputeECS
	}

	if strings.EqualFold(taskMeta.LaunchType, "FARGATE") {
		return ComputeFargate
	}
	return ComputeECS
}

func (d *awsDetector) detectECSOnly(ctx context.Context) *CloudEnvironment {
	ecsType := d.detectECSComputeType(ctx)
	if ecsType == ComputeUnknown {
		return nil
	}

	metadataURI := os.Getenv(ecsMetadataURIV4)
	if metadataURI == "" {
		metadataURI = os.Getenv(ecsMetadataURIV3)
	}

	env := &CloudEnvironment{
		Provider:    ProviderAWS,
		ComputeType: ecsType,
		Metadata:    map[string]string{},
	}

	if metadataURI != "" {
		taskURL := metadataURI + "/task"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, taskURL, nil)
		if err == nil {
			resp, err := d.client.Do(req)
			if err == nil {
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)
				var taskMeta struct {
					TaskARN string `json:"TaskARN"`
				}
				if json.Unmarshal(body, &taskMeta) == nil && taskMeta.TaskARN != "" {
					parts := strings.Split(taskMeta.TaskARN, ":")
					if len(parts) >= 5 {
						env.Region = parts[3]
						env.AccountID = parts[4]
					}
				}
			}
		}
	}

	return env
}

func (d *awsDetector) getIMDSToken(ctx context.Context, tokenURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "300")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("IMDS token request returned status %d", resp.StatusCode)
	}

	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(token), nil
}

func (d *awsDetector) httpGetWithHeader(ctx context.Context, url, headerKey, headerVal string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set(headerKey, headerVal)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET %s returned status %d", url, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func (d *awsDetector) getVpcID(ctx context.Context, metadataURL, token string) string {
	macBody, err := d.httpGetWithHeader(ctx, metadataURL+"mac", "X-aws-ec2-metadata-token", token)
	if err != nil {
		return ""
	}
	mac := strings.TrimSpace(string(macBody))
	if mac == "" {
		return ""
	}
	mac = strings.Fields(mac)[0]

	vpcBody, err := d.httpGetWithHeader(ctx, metadataURL+"network/interfaces/macs/"+mac+"/vpc-id", "X-aws-ec2-metadata-token", token)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(vpcBody))
}
