package agent

import (
	"github.com/kloudmate/km-agent/internal/config"
	"github.com/kloudmate/km-agent/internal/shared"
	"go.opentelemetry.io/collector/otelcol"
	"go.uber.org/zap"
)

func NewCollector(c *config.Config, logger *zap.SugaredLogger) (*otelcol.Collector, error) {
	enrichedPath, err := shared.EnrichCollectorConfig(c.OtelConfigPath, logger)
	if err != nil {
		logger.Warnw("config enrichent failed, using orginal config", "error", err)
		enrichedPath = c.OtelConfigPath
	}
	collectorSettings := shared.CollectorInfoFactory(enrichedPath)
	return otelcol.NewCollector(collectorSettings)
}
