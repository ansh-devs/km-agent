package clouddetect

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const processorKey = "resourcedetection"

// Updates the "resourcedetection" processor's detectors list with cloud-specific detectors from the detected environment.
// Also ensures any service pipeline that has "hostmetrics" as a receiver includes the "resourcedetection" processor.
// The function is idempotent and safe to call on already-patched configs.
func PatchCollectorConfig(cfg map[string]interface{}, env *CloudEnvironment) map[string]interface{} {
	if env == nil || cfg == nil {
		return cfg
	}

	detectors := env.RecommendedDetectors()
	if len(detectors) == 0 {
		return cfg
	}

	patchProcessorDetectors(cfg, detectors)
	ensureProcessorInHostmetricsPipelines(cfg)

	return cfg
}

// PatchCollectorConfigFile reads a YAML collector config file, patches it with cloud-specific resource detection, and writes it back.
func PatchCollectorConfigFile(path string, env *CloudEnvironment) error {
	if env == nil {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read collector config: %w", err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parse collector config: %w", err)
	}

	cfg = PatchCollectorConfig(cfg, env)

	patched, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal patched config: %w", err)
	}

	if err := os.WriteFile(path, patched, 0644); err != nil {
		return fmt.Errorf("write patched config: %w", err)
	}

	return nil
}

// patchProcessorDetectors updates (or creates) the resourcedetection processor
// with the given detectors list while preserving all other processor settings.
func patchProcessorDetectors(cfg map[string]interface{}, detectors []string) {
	processors := getOrCreateMap(cfg, "processors")

	existing, ok := processors[processorKey]
	if ok {
		if processorMap, ok := asMap(existing); ok {
			// Merge: keep existing config (system, timeout, override, etc.)
			// but replace the detectors list with the full recommended set.
			processorMap["detectors"] = toInterfaceSlice(detectors)
			return
		}
	}

	// Processor doesn't exist yet — create with recommended detectors.
	processors[processorKey] = map[string]interface{}{
		"detectors": toInterfaceSlice(detectors),
		"override":  false,
		"timeout":   "5s",
		"system": map[string]interface{}{
			"hostname_sources": []interface{}{"os"},
			"resource_attributes": map[string]interface{}{
				"host.name": map[string]interface{}{"enabled": true},
				"host.id":   map[string]interface{}{"enabled": true},
			},
		},
	}
}

// ensureProcessorInHostmetricsPipelines scans all service pipelines and ensures
// any pipeline that lists "hostmetrics" in its receivers includes the
// "resourcedetection" processor. If missing, it's prepended to the processor list.
func ensureProcessorInHostmetricsPipelines(cfg map[string]interface{}) {
	service, ok := asMap(cfg["service"])
	if !ok {
		return
	}
	pipelines, ok := asMap(service["pipelines"])
	if !ok {
		return
	}

	for _, pipeline := range pipelines {
		pipelineMap, ok := asMap(pipeline)
		if !ok {
			continue
		}

		if !sliceContainsValue(pipelineMap["receivers"], "hostmetrics") {
			continue
		}

		processors := pipelineMap["processors"]
		if processors == nil {
			pipelineMap["processors"] = []interface{}{processorKey}
			continue
		}

		if sliceContainsValue(processors, processorKey) {
			continue
		}

		// Prepend resourcedetection so it runs first in the pipeline.
		pipelineMap["processors"] = prependToSlice(processors, processorKey)
	}
}

// ---------- helpers ----------

func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if v, ok := parent[key]; ok {
		if m, ok := asMap(v); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	parent[key] = m
	return m
}

// asMap attempts to cast v to map[string]interface{}.
// YAML/JSON unmarshal produces this type for object nodes.
func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

// sliceContainsValue checks if a YAML-unmarshaled slice contains a string value.
func sliceContainsValue(slice interface{}, value string) bool {
	switch s := slice.(type) {
	case []interface{}:
		for _, v := range s {
			if str, ok := v.(string); ok && str == value {
				return true
			}
		}
	case []string:
		for _, v := range s {
			if v == value {
				return true
			}
		}
	}
	return false
}

// prependToSlice inserts value at the beginning of a YAML-unmarshaled slice.
func prependToSlice(slice interface{}, value string) []interface{} {
	result := []interface{}{value}
	switch s := slice.(type) {
	case []interface{}:
		result = append(result, s...)
	case []string:
		for _, v := range s {
			result = append(result, v)
		}
	}
	return result
}

func toInterfaceSlice(ss []string) []interface{} {
	result := make([]interface{}, len(ss))
	for i, s := range ss {
		result[i] = s
	}
	return result
}
