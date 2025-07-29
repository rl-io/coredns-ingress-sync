package logging

import (
	"os"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Setup configures the controller-runtime logger with the specified log level
func Setup() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	
	switch strings.ToLower(logLevel) {
	case "debug":
		// Use development mode for debug, which enables debug logging and more human-readable output
		ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	case "info":
		// Use production mode for info level
		ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	case "warn", "warning", "error":
		// Use production mode for warn/error levels
		ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	default:
		// Default to info level
		ctrl.SetLogger(zap.New(zap.UseDevMode(false)))
	}
}
