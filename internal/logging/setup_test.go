package logging

import (
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
)

func TestSetup(t *testing.T) {
	tests := []struct {
		name     string
		logLevel string
	}{
		{
			name:     "default info level",
			logLevel: "",
		},
		{
			name:     "debug level",
			logLevel: "debug",
		},
		{
			name:     "info level",
			logLevel: "info",
		},
		{
			name:     "warn level",
			logLevel: "warn",
		},
		{
			name:     "error level",
			logLevel: "error",
		},
		{
			name:     "invalid level defaults to info",
			logLevel: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.logLevel != "" {
				t.Setenv("LOG_LEVEL", tt.logLevel)
			}

			// Call setup - should not panic or error
			Setup()

			// Verify logger is set (basic check)
			logger := ctrl.Log.WithName("test")
			// Try to use the logger - should not panic
			logger.Info("test message")
		})
	}
}
