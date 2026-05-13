package clouddetect

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	defaultTimeout = 2 * time.Second
)

// detectorLogger logger for clouddetect extension.
type detectorLogger interface {
	Debugw(msg string, keysAndValues ...interface{})
	Infow(msg string, keysAndValues ...interface{})
	Warnw(msg string, keysAndValues ...interface{})
}

// Detector detects the cloud environment where the agent is running.
type Detector struct {
	logger    detectorLogger
	client    *http.Client
	providers []ProviderDetector

	mu     sync.RWMutex
	cached *CloudEnvironment
}

// NewDetector creates a new cloud environment detector with all providers registered.
func NewDetector(logger *zap.SugaredLogger) *Detector {
	client := &http.Client{Timeout: defaultTimeout}

	d := &Detector{
		logger: logger,
		client: client,
	}

	// Register provider detectors in priority order.
	// Cloud providers with unique metadata endpoints first,
	// then providers sharing 169.254.169.254 (order matters for disambiguation).
	d.providers = []ProviderDetector{
		// AWS (IMDS + ECS env vars + Lambda env vars)
		newAWSDetector(client, "", logger),
		// GCP (metadata.google.internal)
		newGCPDetector(client, "", logger),
		// Azure (169.254.169.254 with Metadata:true header)
		newAzureDetector(client, "", logger),
		// Alibaba Cloud (100.100.100.200 — unique IP, no conflict)
		newAlibabaDetector(client, "", logger),
		// OCI (169.254.169.254/opc/v2/ — unique path)
		newOCIDetector(client, "", logger),
		// DigitalOcean (169.254.169.254/metadata/v1.json)
		newDigitalOceanDetector(client, "", logger),
		// Vultr (169.254.169.254/v1.json)
		newVultrDetector(client, "", logger),
	}

	return d
}

// Detect probes all cloud metadata services and returns the detected environment.
func (d *Detector) Detect(ctx context.Context) *CloudEnvironment {
	d.mu.RLock()
	if d.cached != nil {
		d.mu.RUnlock()
		return d.cached
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cached != nil {
		return d.cached
	}

	env := d.detect(ctx)

	// Always populate OS-level fields
	env.OS = runtime.GOOS
	if hostname, err := os.Hostname(); err == nil {
		env.Hostname = hostname
	}

	// Detect cross-cutting signals (Docker, Hashicorp Consul,RH OpenShift)
	d.detectCrossCuttingSignals(env)

	d.cached = env
	d.logger.Infow("cloud environment detected",
		"provider", env.Provider,
		"compute_type", env.ComputeType,
		"region", env.Region,
		"account_id", env.AccountID,
		"instance_id", env.InstanceID,
		"hostname", env.Hostname,
		"os", env.OS,
		"recommended_detectors", env.RecommendedDetectors(),
	)
	return env
}

// Reset clears the cached detection result, forcing a re-detect on the next call.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cached = nil
}

func (d *Detector) detect(ctx context.Context) *CloudEnvironment {
	for _, provider := range d.providers {
		if env := provider.Detect(ctx); env != nil {
			return env
		}
	}
	return d.detectOnPrem()
}

// detectOnPrem detects the local (non-cloud) environment.
func (d *Detector) detectOnPrem() *CloudEnvironment {
	env := &CloudEnvironment{
		Provider: ProviderOnPrem,
		Metadata: map[string]string{},
	}

	if isKubernetes() {
		env.ComputeType = ComputeKubernetes
		d.logger.Infow("Kubernetes environment detected (non-cloud)")
	} else {
		env.ComputeType = ComputeBareMetal
		d.logger.Infow("no cloud provider detected, classifying as bare metal / on-prem")
	}

	return env
}

// detectCrossCuttingSignals checks for platform signals that are independent
// of the cloud provider (Docker, Consul, OpenShift) and sets metadata flags
// so RecommendedDetectors() can include them.
func (d *Detector) detectCrossCuttingSignals(env *CloudEnvironment) {
	if env.Metadata == nil {
		env.Metadata = map[string]string{}
	}

	// Enable the "docker" detector only when the Docker socket is available
	// AND the agent is actually running inside a Docker container.
	// The docker resource detector calls "docker inspect" on the current
	// container; if the agent runs on a bare host that merely has Docker
	// installed, the inspect call fails fatally.
	if _, err := os.Stat("/var/run/docker.sock"); err == nil && isInsideDockerContainer() {
		env.Metadata["docker"] = "true"
	}

	// check for CONSUL_HTTP_ADDR env var
	if os.Getenv("CONSUL_HTTP_ADDR") != "" {
		env.Metadata["consul"] = "true"
	}

	//  check for OpenShift-specific env vars or API
	if os.Getenv("OPENSHIFT_BUILD_NAME") != "" || os.Getenv("OPENSHIFT_BUILD_NAMESPACE") != "" {
		env.Metadata["openshift"] = "true"
	}
}

// isKubernetes checks if the agent is running inside a Kubernetes cluster.
func isKubernetes() bool {
	if _, err := os.Stat("/var/run/secrets/kubernetes.io/serviceaccount/token"); err == nil {
		return true
	}
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}
	return false
}

// isInsideDockerContainer checks whether the current process is running inside a Docker container
func isInsideDockerContainer() bool {
	// /.dockerenv is created by the Docker runtime inside every container.
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	f, err := os.Open("/proc/self/cgroup")
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "docker") || strings.Contains(line, "containerd") {
			return true
		}
	}
	return false
}
