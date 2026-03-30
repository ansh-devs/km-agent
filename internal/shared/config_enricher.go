package shared

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/kloudmate/km-agent/internal/clouddetect"
)

// toInterfaceSlice converts []string to []interface{} for YAML marshalling.
func toInterfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// pickTimeout returns a longer timeout for cloud environments (metadata
// endpoints can be slow on first call) and a shorter one for on-prem.
func pickTimeout(env *clouddetect.CloudEnvironment) string {
	if env.Provider != clouddetect.ProviderOnPrem {
		return "10s"
	}
	return "5s"
}

// addSystemSubConfig adds the "system" detector configuration block.
func addSystemSubConfig(rd map[string]interface{}) {
	rd["system"] = map[string]interface{}{
		"hostname_sources": []interface{}{"os"},
		"resource_attributes": map[string]interface{}{
			"host.name": map[string]interface{}{"enabled": true},
			"host.id":   map[string]interface{}{"enabled": true},
			"os.type":   map[string]interface{}{"enabled": true},
		},
	}
}

// addK8sNodeSubConfig adds the "k8snode" detector configuration block.
func addK8sNodeSubConfig(rd map[string]interface{}) {
	rd["k8snode"] = map[string]interface{}{
		"node_from_env_var": "KM_NODE_NAME",
		"resource_attributes": map[string]interface{}{
			"k8s.node.uid":  map[string]interface{}{"enabled": true},
			"k8s.node.name": map[string]interface{}{"enabled": true},
		},
	}
}

// addAWSSubConfig adds AWS-specific detector configs based on compute type.
func addAWSSubConfig(rd map[string]interface{}, computeType clouddetect.ComputeType) {
	switch computeType {
	case clouddetect.ComputeEC2, clouddetect.ComputeElasticBeanstalk:
		rd["ec2"] = map[string]interface{}{
			"resource_attributes": map[string]interface{}{
				"cloud.account.id":        map[string]interface{}{"enabled": true},
				"cloud.availability_zone": map[string]interface{}{"enabled": true},
				"cloud.platform":          map[string]interface{}{"enabled": true},
				"cloud.provider":          map[string]interface{}{"enabled": true},
				"cloud.region":            map[string]interface{}{"enabled": true},
				"host.id":                 map[string]interface{}{"enabled": true},
				"host.image.id":           map[string]interface{}{"enabled": true},
				"host.name":               map[string]interface{}{"enabled": true},
				"host.type":               map[string]interface{}{"enabled": true},
			},
		}

	case clouddetect.ComputeEKS:
		rd["ec2"] = map[string]interface{}{
			"resource_attributes": map[string]interface{}{
				"cloud.account.id":        map[string]interface{}{"enabled": true},
				"cloud.availability_zone": map[string]interface{}{"enabled": true},
				"cloud.platform":          map[string]interface{}{"enabled": true},
				"cloud.provider":          map[string]interface{}{"enabled": true},
				"cloud.region":            map[string]interface{}{"enabled": true},
				"host.id":                 map[string]interface{}{"enabled": true},
				"host.image.id":           map[string]interface{}{"enabled": true},
				"host.name":               map[string]interface{}{"enabled": true},
				"host.type":               map[string]interface{}{"enabled": true},
			},
		}
		rd["eks"] = map[string]interface{}{
			"resource_attributes": map[string]interface{}{
				"k8s.cluster.name": map[string]interface{}{"enabled": true},
			},
		}
		addK8sNodeSubConfig(rd)

	case clouddetect.ComputeECS, clouddetect.ComputeFargate:
		rd["ecs"] = map[string]interface{}{
			"resource_attributes": map[string]interface{}{
				"aws.ecs.cluster.arn":     map[string]interface{}{"enabled": true},
				"aws.ecs.launchtype":      map[string]interface{}{"enabled": true},
				"aws.ecs.task.arn":        map[string]interface{}{"enabled": true},
				"aws.ecs.task.family":     map[string]interface{}{"enabled": true},
				"aws.ecs.task.id":         map[string]interface{}{"enabled": true},
				"aws.ecs.task.revision":   map[string]interface{}{"enabled": true},
				"cloud.account.id":        map[string]interface{}{"enabled": true},
				"cloud.availability_zone": map[string]interface{}{"enabled": true},
				"cloud.platform":          map[string]interface{}{"enabled": true},
				"cloud.provider":          map[string]interface{}{"enabled": true},
				"cloud.region":            map[string]interface{}{"enabled": true},
				"aws.log.group.arns":      map[string]interface{}{"enabled": true},
				"aws.log.group.names":     map[string]interface{}{"enabled": true},
				"aws.log.stream.arns":     map[string]interface{}{"enabled": true},
				"aws.log.stream.names":    map[string]interface{}{"enabled": true},
			},
		}

	case clouddetect.ComputeLambda:
		rd["lambda"] = map[string]interface{}{
			"resource_attributes": map[string]interface{}{
				"cloud.platform":  map[string]interface{}{"enabled": true},
				"cloud.provider":  map[string]interface{}{"enabled": true},
				"cloud.region":    map[string]interface{}{"enabled": true},
				"faas.instance":   map[string]interface{}{"enabled": true},
				"faas.max_memory": map[string]interface{}{"enabled": true},
				"faas.name":       map[string]interface{}{"enabled": true},
				"faas.version":    map[string]interface{}{"enabled": true},
			},
		}
	}
}

// addGCPSubConfig adds the "gcp" detector configuration block.
func addGCPSubConfig(rd map[string]interface{}, computeType clouddetect.ComputeType) {
	rd["gcp"] = map[string]interface{}{
		"resource_attributes": map[string]interface{}{
			"cloud.account.id":        map[string]interface{}{"enabled": true},
			"cloud.availability_zone": map[string]interface{}{"enabled": true},
			"cloud.platform":          map[string]interface{}{"enabled": true},
			"cloud.provider":          map[string]interface{}{"enabled": true},
			"host.id":                 map[string]interface{}{"enabled": true},
			"host.name":               map[string]interface{}{"enabled": true},
			"host.type":               map[string]interface{}{"enabled": true},
		},
	}
	if computeType == clouddetect.ComputeGKE {
		addK8sNodeSubConfig(rd)
	}
}

// addAzureSubConfig adds the "azure" detector configuration block.
func addAzureSubConfig(rd map[string]interface{}, computeType clouddetect.ComputeType) {
	rd["azure"] = map[string]interface{}{
		"resource_attributes": map[string]interface{}{
			"azure.resourcegroup.name": map[string]interface{}{"enabled": true},
			"azure.vm.name":            map[string]interface{}{"enabled": true},
			"azure.vm.scaleset.name":   map[string]interface{}{"enabled": true},
			"azure.vm.size":            map[string]interface{}{"enabled": true},
			"cloud.account.id":         map[string]interface{}{"enabled": true},
			"cloud.platform":           map[string]interface{}{"enabled": true},
			"cloud.provider":           map[string]interface{}{"enabled": true},
			"cloud.region":             map[string]interface{}{"enabled": true},
			"host.id":                  map[string]interface{}{"enabled": true},
			"host.name":                map[string]interface{}{"enabled": true},
		},
	}
	if computeType == clouddetect.ComputeAKS {
		addK8sNodeSubConfig(rd)
	}
}

// GenerateResourceDetectionBlock builds the "processors.resourcedetection"
// config map for the detected CloudEnvironment
func GenerateResourceDetectionBlock(env *clouddetect.CloudEnvironment) map[string]interface{} {
	detectors := env.RecommendedDetectors()

	rd := map[string]interface{}{
		"detectors": toInterfaceSlice(detectors),
		"timeout":   pickTimeout(env),
		"override":  false,
	}

	// system detector config is always included
	addSystemSubConfig(rd)

	// Provider-specific detector sub-configs
	switch env.Provider {
	case clouddetect.ProviderAWS:
		addAWSSubConfig(rd, env.ComputeType)

	case clouddetect.ProviderGCP:
		addGCPSubConfig(rd, env.ComputeType)

	case clouddetect.ProviderAzure:
		addAzureSubConfig(rd, env.ComputeType)

	case clouddetect.ProviderOnPrem:
		if env.ComputeType == clouddetect.ComputeKubernetes {
			addK8sNodeSubConfig(rd)
		}
	}

	return map[string]interface{}{
		"processors": map[string]interface{}{
			"resourcedetection": rd,
		},
	}
}

// deepMergeMap recursively merges override into base and returns the result.
// For map values that exist in both, the merge recurses.
// For all other types (scalars, slices), the override value wins.
func deepMergeMap(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, ov := range override {
		bv, exists := result[k]
		if exists {
			bMap, bIsMap := bv.(map[string]interface{})
			oMap, oIsMap := ov.(map[string]interface{})
			if bIsMap && oIsMap {
				result[k] = deepMergeMap(bMap, oMap)
				continue
			}
		}
		result[k] = ov
	}
	return result
}

// Merge semantics: our generated block is the base; the user's file values
// take precedence (override). Any key the user explicitly sets in their
// YAML is preserved as-is.

// @amitava82: overwritten on every call (safe for collector restarts).
func EnrichCollectorConfig(configPath string, logger *zap.SugaredLogger) (string, error) {
	// --- detect cloud environment ---
	detector := clouddetect.NewDetector(logger)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	env := detector.Detect(ctx)

	logger.Infow("enriching collector config with resourcedetection block",
		"configPath", configPath,
		"provider", env.Provider,
		"computeType", env.ComputeType,
		"recommendedDetectors", env.RecommendedDetectors(),
	)

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read collector config %q: %w", configPath, err)
	}

	var userConfig map[string]interface{}
	if err := yaml.Unmarshal(data, &userConfig); err != nil {
		return "", fmt.Errorf("parse collector config %q: %w", configPath, err)
	}
	if userConfig == nil {
		userConfig = make(map[string]interface{})
	}

	generated := GenerateResourceDetectionBlock(env)
	merged := deepMergeMap(generated, userConfig)

	out, err := yaml.Marshal(merged)
	if err != nil {
		return "", fmt.Errorf("marshal enriched config: %w", err)
	}

	enrichedPath := filepath.Join(os.TempDir(), "km-sugar-coated-config.yaml")
	if err := os.WriteFile(enrichedPath, out, 0644); err != nil {
		return "", fmt.Errorf("write enriched config to %q: %w", enrichedPath, err)
	}

	return enrichedPath, nil
}
