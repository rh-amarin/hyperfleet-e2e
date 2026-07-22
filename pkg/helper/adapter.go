package helper

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type AdapterDeployment struct {
	ReleaseName  string
	AdapterName  string
	ResourceType string
}

type AdapterDeploymentList struct {
	mu    sync.RWMutex
	items []AdapterDeployment
}

func (l *AdapterDeploymentList) Add(deployment AdapterDeployment) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, deployment)
}

// Snapshot returns a thread-safe copy of all adapter deployments
func (l *AdapterDeploymentList) Snapshot() []AdapterDeployment {
	l.mu.RLock()
	defer l.mu.RUnlock()
	snapshot := make([]AdapterDeployment, len(l.items))
	copy(snapshot, l.items)
	return snapshot
}

func InitAdapterDeploymentList() *AdapterDeploymentList {
	return &AdapterDeploymentList{
		items: make([]AdapterDeployment, 0),
	}
}

// generateRandomString generates a random alphanumeric string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback: use current time nanoseconds for basic randomness
			b[i] = charset[(time.Now().UnixNano()+int64(i))%int64(len(charset))]
		} else {
			b[i] = charset[n.Int64()]
		}
	}
	return string(b)
}

// AdapterDeploymentOptions contains configuration for deploying an adapter via Helm
type AdapterDeploymentOptions struct {
	ReleaseName  string
	Namespace    string
	ChartPath    string
	AdapterName  string
	Timeout      time.Duration
	SetValues    map[string]string // Additional Helm --set values
	ResourceType string
}

// GenerateAdapterReleaseName generates a deterministic Helm release name for an adapter deployment.
// The release name format is: adapter-<resource_type>-<adapter_name>
// Deterministic naming allows helm upgrade --install to upgrade in place and avoids duplicate releases.
// The name is truncated to 48 characters to leave room for Helm's deployment/pod suffixes (Kubernetes has a 63-char limit).
// If truncation is needed, a deterministic hash is appended to prevent collisions between long names.
const maxReleaseNameLength = 48

func GenerateAdapterReleaseName(resourceType, adapterName string) string {
	releaseName := fmt.Sprintf("adapter-%s-%s", resourceType, adapterName)

	if len(releaseName) > maxReleaseNameLength {
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(releaseName)))[:8]
		truncLen := maxReleaseNameLength - len(hash) - 1
		releaseName = releaseName[:truncLen] + "-" + hash
	}

	return releaseName
}

// DeployAdapter deploys an adapter using Helm upgrade --install
// This is a common function that can be reused across test cases
// The release name must be provided via opts.ReleaseName - use GenerateAdapterReleaseName() to create a unique name
func (h *Helper) DeployAdapter(ctx context.Context, opts AdapterDeploymentOptions) error {
	// Validate required fields
	if opts.Namespace == "" {
		return fmt.Errorf("AdapterDeploymentOptions.Namespace is required")
	}
	if opts.ChartPath == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ChartPath is required")
	}
	if opts.AdapterName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.AdapterName is required")
	}
	if opts.ReleaseName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ReleaseName is required - use GenerateAdapterReleaseName() to create a unique name")
	}

	// Set default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	releaseName := opts.ReleaseName

	logger.Info("deploying adapter via Helm",
		"adapter_name", opts.AdapterName,
		"release_name", releaseName,
		"namespace", opts.Namespace)

	// Copy adapter config folder to chart directory
	sourceAdapterDir := filepath.Join(h.Cfg.TestDataDir, AdapterConfigsDir, opts.AdapterName)
	destAdapterDir := filepath.Join(opts.ChartPath, opts.AdapterName)

	// Remove existing adapter config directory if it exists
	if _, err := os.Stat(destAdapterDir); err == nil {
		logger.Info("removing existing adapter config directory", "path", destAdapterDir)
		if err := os.RemoveAll(destAdapterDir); err != nil {
			return fmt.Errorf("failed to remove existing adapter config directory: %w", err)
		}
	}

	// Copy adapter config directory to chart
	logger.Info("copying adapter config", "from", sourceAdapterDir, "to", destAdapterDir)
	if err := copyDir(sourceAdapterDir, destAdapterDir); err != nil {
		return fmt.Errorf("failed to copy adapter config directory: %w", err)
	}

	// Determine the values.yaml file path in the copied adapter directory
	valuesFilePath := filepath.Join(destAdapterDir, "values.yaml")

	// Default BROKER_TYPE to googlepubsub if not set so envsubst produces a valid value
	if os.Getenv("BROKER_TYPE") == "" {
		if err := os.Setenv("BROKER_TYPE", "googlepubsub"); err != nil {
			return fmt.Errorf("failed to set default BROKER_TYPE: %w", err)
		}
		defer func() { _ = os.Unsetenv("BROKER_TYPE") }()
	}

	// Compute extra environment variables for the envsubst subprocess.
	// These are scoped to the subprocess and do not mutate the process-global environment.
	var extraEnv []string

	// When using GCP Pub/Sub, ensure the subscription is created if it doesn't exist.
	// This is required for adapters deployed for the first time (no pre-existing subscription).
	if os.Getenv("BROKER_TYPE") == "googlepubsub" && os.Getenv("ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING") == "" {
		extraEnv = append(extraEnv, "ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING=true")
	}

	// Resolve the in-cluster HyperFleet API URL for adapters running inside Kubernetes.
	// The external LoadBalancer IP (HYPERFLEET_API_URL) is not routable from within GKE pods.
	// We look up the hyperfleet-api service across all namespaces and construct the FQDN so
	// that adapters deployed to the test namespace can reach the API regardless of where it runs.
	if os.Getenv("ADAPTER_HYPERFLEET_API_URL") == "" && h.K8sClient != nil {
		if internalURL, err := h.resolveInternalAPIURL(ctx); err == nil && internalURL != "" {
			extraEnv = append(extraEnv, "ADAPTER_HYPERFLEET_API_URL="+internalURL)
			logger.Info("resolved in-cluster HyperFleet API URL for adapters", "url", internalURL)
		} else {
			logger.Info("could not resolve in-cluster API URL, falling back to HYPERFLEET_API_URL",
				"error", err)
		}
	}

	// Expand environment variables in values.yaml in-place using envsubst
	logger.Info("expanding environment variables in values.yaml in-place", "values_file", valuesFilePath)

	expandedContent, err := expandEnvVarsInYAMLToBytes(ctx, valuesFilePath, extraEnv)
	if err != nil {
		return fmt.Errorf("failed to expand environment variables in values.yaml: %w", err)
	}
	if err := os.WriteFile(valuesFilePath, expandedContent, 0600); err != nil {
		return fmt.Errorf("failed to overwrite values.yaml with expanded content: %w", err)
	}

	logger.Info("successfully expanded environment variables in values.yaml")

	// Expand environment variables in adapter-config.yaml in-place using envsubst.
	// This allows adapter configs to reference env vars like ${HYPERFLEET_API_URL}
	// so the correct API endpoint is injected at deploy time regardless of namespace.
	adapterConfigPath := filepath.Join(destAdapterDir, "adapter-config.yaml")
	if _, statErr := os.Stat(adapterConfigPath); statErr == nil {
		expandedAdapterConfig, err := expandEnvVarsInYAMLToBytes(ctx, adapterConfigPath, extraEnv)
		if err != nil {
			return fmt.Errorf("failed to expand environment variables in adapter-config.yaml: %w", err)
		}
		if err := os.WriteFile(adapterConfigPath, expandedAdapterConfig, 0600); err != nil {
			return fmt.Errorf("failed to overwrite adapter-config.yaml with expanded content: %w", err)
		}
		logger.Info("successfully expanded environment variables in adapter-config.yaml")
	}

	// Build Helm command with values file
	helmArgs := []string{
		"upgrade", "--install",
		releaseName,
		opts.ChartPath,
		"--namespace", opts.Namespace,
		"--create-namespace",
		"--wait",
		"--timeout", opts.Timeout.String(),
		"-f", valuesFilePath,
	}

	// Append conditional --set flags
	helmArgs = append(helmArgs, h.adapterHelmSetArgs(releaseName, opts)...)

	logger.Info("executing Helm command", "args", helmArgs)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, opts.Timeout+30*time.Second)
	defer cancel()

	// Execute Helm command
	cmd := exec.CommandContext(cmdCtx, "helm", helmArgs...) // #nosec G204 -- helmArgs is constructed from trusted config
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("helm upgrade failed", "error", err, "output", string(output))

		// Collect diagnostic information when deployment fails
		h.saveDiagnosticLogs(ctx, opts.AdapterName, releaseName, opts.Namespace)

		return fmt.Errorf("helm upgrade failed: %w (output: %s)", err, string(output))
	}

	// Add adapter deployment to list for cleanup
	h.AdapterDeploymentList.Add(AdapterDeployment{
		ReleaseName:  releaseName,
		AdapterName:  opts.AdapterName,
		ResourceType: opts.ResourceType,
	})

	logger.Info("adapter deployed successfully",
		"release_name", releaseName,
		"output", string(output))

	return nil
}

// adapterHelmSetArgs builds the conditional --set flags for adapter Helm deployments.
// Extracted for testability - DeployAdapter calls this to append flags after the base args.
func (h *Helper) adapterHelmSetArgs(releaseName string, opts AdapterDeploymentOptions) []string {
	var args []string

	// Ensure consistent release naming
	args = append(args, "--set", fmt.Sprintf("fullnameOverride=%s", releaseName))

	// Add run-id label for resource tracking and cleanup
	if h.Cfg.RunID != "" {
		args = append(args, "--labels", fmt.Sprintf("e2e.hyperfleet.io/run-id=%s", h.Cfg.RunID))
	}

	// Override image pull policy if set (e.g. IfNotPresent for local kind clusters)
	if policy := os.Getenv("IMAGE_PULL_POLICY"); policy != "" {
		args = append(args, "--set", fmt.Sprintf("image.pullPolicy=%s", policy))
	}

	// Enable adapter API auth when JWT is enabled on the API server
	if h.Cfg.Identity.TokenRequest.IsEnabled() {
		args = append(args, "--set", "adapterConfig.hyperfleetApi.auth.enabled=true")
	}

	// Add additional --set values if provided
	for key, value := range opts.SetValues {
		args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
	}

	return args
}

// resolveInternalAPIURL looks up the hyperfleet-api Kubernetes service in the configured
// namespace and returns an in-cluster FQDN URL that adapters deployed in any namespace can use.
// This is needed because the external LoadBalancer IP is not routable from within GKE pods.
func (h *Helper) resolveInternalAPIURL(ctx context.Context) (string, error) {
	ns := h.Cfg.Namespace
	svc, err := h.K8sClient.CoreV1().Services(ns).Get(ctx, "hyperfleet-api", metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get hyperfleet-api service in namespace %q: %w", ns, err)
	}
	if len(svc.Spec.Ports) == 0 {
		return "", fmt.Errorf("hyperfleet-api service has no ports")
	}
	port := svc.Spec.Ports[0].Port
	return fmt.Sprintf("http://hyperfleet-api.%s.svc.cluster.local:%d", ns, port), nil
}

// UninstallAdapter uninstalls an adapter using Helm uninstall
// This is a common function that can be reused across test cases
func (h *Helper) UninstallAdapter(ctx context.Context, releaseName, namespace string) error {
	logger.Info("uninstalling adapter via Helm",
		"release_name", releaseName,
		"namespace", namespace)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Execute Helm uninstall command
	cmd := exec.CommandContext(cmdCtx, "helm", "uninstall", releaseName,
		"-n", namespace,
		"--wait",
		"--timeout", "5m")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is because the release doesn't exist
		if strings.Contains(string(output), "not found") {
			logger.Info("adapter release not found, skipping uninstall", "release_name", releaseName)
			// Clean up orphaned cluster-scoped resources even when release is not found
			// This handles cases like interrupted installs or manual deletions
			h.cleanupClusterScopedResources(ctx, releaseName)
			return nil
		}
		logger.Error("helm uninstall failed", "error", err, "output", string(output))
		return fmt.Errorf("helm uninstall failed: %w (output: %s)", err, string(output))
	}

	logger.Info("adapter uninstalled successfully",
		"release_name", releaseName,
		"output", string(output))

	// Clean up any orphaned cluster-scoped resources (ClusterRoles, ClusterRoleBindings)
	// These can be left behind if a previous test run failed or was interrupted
	h.cleanupClusterScopedResources(ctx, releaseName)

	return nil
}

// cleanupClusterScopedResources removes orphaned cluster-scoped resources that may be left
// after Helm uninstall. This is a best-effort cleanup and logs errors without failing.
// Uses label selectors instead of names so it works regardless of the chart's naming scheme.
func (h *Helper) cleanupClusterScopedResources(ctx context.Context, releaseName string) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	labelSelector := fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName)

	// Try to delete ClusterRole
	clusterRoleCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrole", //nolint:gosec // labelSelector is derived from releaseName, not user input
		"-l", labelSelector,
		"--ignore-not-found=true")
	if output, err := clusterRoleCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRole (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRole", "release_name", releaseName)
	}

	// Try to delete ClusterRoleBinding
	clusterRoleBindingCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrolebinding", //nolint:gosec // labelSelector is derived from releaseName, not user input
		"-l", labelSelector,
		"--ignore-not-found=true")
	if output, err := clusterRoleBindingCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRoleBinding (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRoleBinding", "release_name", releaseName)
	}
}

// saveDiagnosticLogs saves diagnostic information when adapter deployment fails
// Saves to <outputDir>/<adapter-name>-<random-4chars>/ directory
// outputDir is configured via OUTPUT_DIR env var or config file (defaults to "output")
func (h *Helper) saveDiagnosticLogs(ctx context.Context, adapterName, releaseName, namespace string) {
	// Generate output directory with adapter name and random suffix
	randomSuffix := generateRandomString(4)
	outputDir := filepath.Join(h.Cfg.OutputDir, fmt.Sprintf("%s-%s", adapterName, randomSuffix))

	// Create output directory
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		logger.Error("failed to create diagnostic output directory",
			"error", err,
			"output_dir", outputDir)
		return
	}

	logger.Info("saving diagnostic logs",
		"adapter_name", adapterName,
		"release_name", releaseName,
		"namespace", namespace,
		"output_dir", outputDir)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Get pods using client-go
	pods, err := h.K8sClient.CoreV1().Pods(namespace).List(cmdCtx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		logger.Error("failed to list pods", "error", err)
		return
	}

	if len(pods.Items) == 0 {
		logger.Info("no pods found for release", "release_name", releaseName)
		return
	}

	logger.Info("found pods for release",
		"total_pods", len(pods.Items),
		"release_name", releaseName)

	// Save logs and description for unhealthy pods only
	for _, pod := range pods.Items {
		// Check if pod is healthy (Running and all containers ready)
		isHealthy := pod.Status.Phase == "Running"
		if isHealthy && len(pod.Status.ContainerStatuses) > 0 {
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					isHealthy = false
					break
				}
			}
		}

		// Skip healthy pods
		if isHealthy {
			logger.Info("skipping healthy pod", "pod", pod.Name)
			continue
		}

		podName := pod.Name
		logger.Info("saving logs for unhealthy pod",
			"pod", podName,
			"phase", pod.Status.Phase)

		// Save pod logs using kubectl command
		podLogFile := filepath.Join(outputDir, fmt.Sprintf("%s.log", podName))
		podLogCmd := exec.CommandContext(cmdCtx, "kubectl", "logs", // #nosec G204 -- podName and namespace are from trusted k8s API
			podName,
			"-n", namespace,
			"--tail=200")

		var logContent string
		logContent += fmt.Sprintf("$ %s\n\n", podLogCmd.String())
		logOutput, err := podLogCmd.CombinedOutput()
		if err != nil {
			logContent += fmt.Sprintf("Error: %v\n", err)
			logContent += string(logOutput)
		} else {
			logContent += string(logOutput)
		}

		if err := os.WriteFile(podLogFile, []byte(logContent), 0600); err != nil {
			logger.Error("failed to write pod log file",
				"pod", podName,
				"error", err)
		} else {
			logger.Info("saved pod logs",
				"pod", podName,
				"file", podLogFile)
		}

		// Save pod description using kubectl describe command
		podDescFile := filepath.Join(outputDir, fmt.Sprintf("%s-describe.txt", podName))
		podDescCmd := exec.CommandContext(cmdCtx, "kubectl", "describe", "pod", // #nosec G204 -- podName and namespace are from trusted k8s API
			podName,
			"-n", namespace)

		var descContent string
		descContent += fmt.Sprintf("$ %s\n\n", podDescCmd.String())
		descOutput, err := podDescCmd.CombinedOutput()
		if err != nil {
			descContent += fmt.Sprintf("Error: %v\n", err)
			descContent += string(descOutput)
		} else {
			descContent += string(descOutput)
		}

		if err := os.WriteFile(podDescFile, []byte(descContent), 0600); err != nil {
			logger.Error("failed to write pod description file",
				"pod", podName,
				"error", err)
		} else {
			logger.Info("saved pod description",
				"pod", podName,
				"file", podDescFile)
		}
	}

	logger.Info("diagnostic logs saved successfully", "output_dir", outputDir)
}

// expandEnvVarsInYAMLToBytes expands environment variables in a YAML file using envsubst
// Returns the expanded content as bytes
func expandEnvVarsInYAMLToBytes(ctx context.Context, yamlPath string, extraEnv []string) ([]byte, error) {
	// Read the YAML file
	content, err := os.ReadFile(yamlPath) // #nosec G304 -- yamlPath is constructed from trusted config
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// Use envsubst command to expand environment variables
	cmd := exec.CommandContext(ctx, "envsubst")
	cmd.Stdin = bytes.NewReader(content)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("envsubst failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}


// copyDir recursively copies a directory tree
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcData, err := os.ReadFile(src) // #nosec G304 -- src is constructed from trusted config
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, srcData, srcInfo.Mode())
}
