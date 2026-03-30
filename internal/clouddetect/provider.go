package clouddetect

import (
	"context"
	"net/http"
)

// ProviderDetector is the interface that each cloud provider implements
type ProviderDetector interface {
	// Name returns the provider identifier (e.g., "aws", "gcp", "azure").
	Name() Provider

	// Detect probes the provider's metadata service
	Detect(ctx context.Context) *CloudEnvironment
}

// baseDetector holds common fields shared by all provider detectors.
type baseDetector struct {
	client  *http.Client
	baseURL string
}
