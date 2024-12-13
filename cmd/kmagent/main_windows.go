//go:build windows

package main

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"

	"go.opentelemetry.io/collector/otelcol"
)

func run(params otelcol.CollectorSettings) error {
	// No need to supply service name when startup is invoked through
	// the Service Control Manager directly.
	if err := svc.Run("", otelcol.NewSvcHandler(params)); err != nil {
		if errors.Is(err, windows.ERROR_FAILED_SERVICE_CONTROLLER_CONNECT) {
			// Per https://learn.microsoft.com/en-us/windows/win32/api/winsvc/nf-winsvc-startservicectrldispatchera#return-value
			// this means that the process is not running as a service, so run interactively.
			return runInteractive(params)
		}

		return fmt.Errorf("failed to start collector server: %w", err)
	}

	return nil
}