package config

import "time"

// Log level constants
const (
	// LogLevelDebug enables detailed test steps and all framework internal logs
	LogLevelDebug = "debug"

	// LogLevelInfo enables detailed test steps and high-level framework logs (default)
	LogLevelInfo = "info"

	// LogLevelWarn shows only warnings and errors (minimal output for CI/CD)
	LogLevelWarn = "warn"

	// LogLevelError shows only errors (absolute minimal output)
	LogLevelError = "error"
)

// Log format constants
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// Log output constants
const (
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
)

// Default timeout values
const (
	// DefaultClusterReconciledTimeout is the default timeout for waiting for a cluster to become reconciled
	DefaultClusterReconciledTimeout = 2 * time.Minute

	// DefaultClusterDeletedTimeout is the default timeout for waiting for a cluster to be hard-deleted (404)
	DefaultClusterDeletedTimeout = 2 * time.Minute

	// DefaultNodePoolReconciledTimeout is the default timeout for waiting for a nodepool to become reconciled
	DefaultNodePoolReconciledTimeout = 5 * time.Minute

	// DefaultNodePoolDeletedTimeout is the default timeout for waiting for a nodepool to be hard-deleted (404)
	DefaultNodePoolDeletedTimeout = 2 * time.Minute

	// DefaultAdapterProcessingTimeout is the default timeout for waiting for adapter conditions
	DefaultAdapterProcessingTimeout = 5 * time.Minute

	// DefaultPollInterval is the default interval for polling operations
	DefaultPollInterval = 10 * time.Second

	// DefaultLogLevel is the default log level
	DefaultLogLevel = LogLevelInfo

	// DefaultLogFormat is the default log format
	DefaultLogFormat = LogFormatText

	// DefaultLogOutput is the default log output
	DefaultLogOutput = LogOutputStdout

	// DefaultTokenRequestExpirationSeconds is the default lifetime for TokenRequest tokens (matches suite timeout)
	DefaultTokenRequestExpirationSeconds int64 = 7200

	// DefaultTokenRequestAudience is the default audience for TokenRequest tokens
	DefaultTokenRequestAudience = "hyperfleet-api"
)

// Default required adapters for resource types as name→URL maps.
// These defaults are typically overridden by configs/config.yaml.
var (
	// DefaultClusterAdapters is the default map of required adapters for cluster resources
	DefaultClusterAdapters = map[string]string{
		"cl-namespace":  "http://cl-namespace.hyperfleet.svc.cluster.local:8082",
		"cl-job":        "http://cl-job.hyperfleet.svc.cluster.local:8082",
		"cl-deployment": "http://cl-deployment.hyperfleet.svc.cluster.local:8082",
		"cl-maestro":    "http://cl-maestro.hyperfleet.svc.cluster.local:8082",
	}

	// DefaultNodePoolAdapters is the default map of required adapters for nodepool resources
	DefaultNodePoolAdapters = map[string]string{
		"np-configmap": "http://np-configmap.hyperfleet.svc.cluster.local:8082",
	}
)
