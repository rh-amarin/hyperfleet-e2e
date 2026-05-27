package tui

import (
	"slices"
	"strings"
)

// knownEnvVars lists environment variables shown in the TUI sidebar and passed to
// test subprocesses. Defaults mirror deploy-scripts/.env.example where applicable.
var knownEnvVars = []EnvVar{
	// E2E API / Maestro (required for most tests)
	{Key: "HYPERFLEET_API_URL", Default: "http://localhost:8000"},
	{Key: "MAESTRO_URL", Default: "http://localhost:8100"},

	// Kubernetes / deployment target
	{Key: "NAMESPACE", Default: "hyperfleet-e2e-xxx"},
	{Key: "KUBECONFIG", Default: ""},

	// Broker (local kind + RabbitMQ)
	{Key: "BROKER_TYPE", Default: "googlepubsub"},
	{Key: "RABBITMQ_URL", Default: "amqp://guest:guest@rabbitmq.rabbitmq:5672"},
	{Key: "GCP_PROJECT_ID", Default: "hcm-hyperfleet"},

	// Images
	{Key: "IMAGE_REGISTRY", Default: "quay.io/redhat-services-prod/"},
	{Key: "ADAPTER_IMAGE_REPO", Default: "hyperfleet-adapter"},
	{Key: "ADAPTER_IMAGE_TAG", Default: "latest"},
	{Key: "IMAGE_PULL_POLICY", Default: "IfNotPresent"},

	// API adapter lists (config + deploy)
	{Key: "API_ADAPTERS_CLUSTER", Default: "cl-namespace,cl-job,cl-deployment,cl-maestro"},
	{Key: "API_ADAPTERS_NODEPOOL", Default: "np-configmap"},
	{Key: "CLUSTER_TIER0_ADAPTERS_DEPLOYMENT", Default: "cl-namespace,cl-job,cl-deployment,cl-maestro"},
	{Key: "NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT", Default: "np-configmap"},

	// Helm charts (tier2 / adapter deploy tests)
	{Key: "ADAPTER_CHART_REPO", Default: "https://github.com/openshift-hyperfleet/hyperfleet-adapter.git"},
	{Key: "ADAPTER_CHART_REF", Default: "main"},
	{Key: "ADAPTER_CHART_PATH", Default: "charts"},
	{Key: "API_CHART_REPO", Default: "https://github.com/openshift-hyperfleet/hyperfleet-api.git"},
	{Key: "API_CHART_REF", Default: "main"},
	{Key: "API_CHART_PATH", Default: "charts"},

	// Pub/Sub adapter options (GCP; harmless when using RabbitMQ)
	{Key: "ADAPTER_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING", Default: "true"},
	{Key: "ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING", Default: "true"},

	// Test data
	{Key: "TESTDATA_DIR", Default: "testdata"},
}

func sortEnvVarsByKey(vars []EnvVar) {
	slices.SortFunc(vars, func(a, b EnvVar) int {
		return strings.Compare(a.Key, b.Key)
	})
}
