package helper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

const (
	entityKindCluster  = "Cluster"
	helmReleaseName    = "hyperfleet-api"
	helmUpgradeTimeout = 3 * time.Minute
	// Give the context headroom over helm's own --timeout so helm can
	// report its own error instead of being killed by context cancellation.
	helmContextTimeout = helmUpgradeTimeout + 30*time.Second
	apiReadyTimeout    = 2 * time.Minute
	apiReadyInterval   = 2 * time.Second
	retryBackoff       = 10 * time.Second
)

// apiValues is the subset of Helm values we need to read-modify-write.
// The inline catch-alls preserve every field we don't model on the round-trip.
type apiValues struct {
	Config struct {
		Entities []apiEntity    `yaml:"entities"`
		Rest     map[string]any `yaml:",inline"`
	} `yaml:"config"`
	Rest map[string]any `yaml:",inline"`
}

type apiEntity struct {
	Kind             string            `yaml:"kind"`
	RequiredAdapters map[string]string `yaml:"required_adapters"`
	Rest             map[string]any    `yaml:",inline"`
}

// UpgradeAPIRequiredAdapters upgrades the API Helm release to update the
// required adapters for the Cluster entity. It reads the current Helm values,
// patches the correct entity by kind (not by array index), and applies.
func (h *Helper) UpgradeAPIRequiredAdapters(ctx context.Context, apiChartPath, namespace string, clusterAdapters map[string]string) error {
	logger.Info("upgrading API required adapters",
		"namespace", namespace,
		"cluster_adapters", clusterAdapters)

	cmdCtx, cancel := context.WithTimeout(ctx, helmContextTimeout)
	defer cancel()

	patchedBytes, err := getAndPatchHelmValues(cmdCtx, namespace, entityKindCluster, clusterAdapters)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp("", "api-values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if err := os.Remove(tmpPath); err != nil {
			logger.Info("failed to remove temp values file", "path", tmpPath, "error", err)
		}
	}()

	if _, err := tmpFile.Write(patchedBytes); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed to write temp values file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp values file: %w", err)
	}

	// --values with the full value set (from --all) instead of --reuse-values
	// makes the applied config explicit and avoids merging surprises.
	upgradeCmd := exec.CommandContext(cmdCtx, "helm", "upgrade", helmReleaseName, apiChartPath, // #nosec G204
		"--namespace", namespace,
		"--values", tmpPath,
		"--wait",
		"--timeout", helmUpgradeTimeout.String(),
	)

	output, err := upgradeCmd.CombinedOutput()
	if err != nil {
		logger.Error("helm upgrade API failed", "error", err, "output", string(output))
		return fmt.Errorf("failed to upgrade API: %w (output: %s)", err, string(output))
	}

	logger.Info("API required adapters updated successfully",
		"cluster_adapters", clusterAdapters,
		"output", string(output))

	return h.waitForAPIReady(ctx)
}

// waitForAPIReady polls the API until it responds or the timeout expires.
// Uses a plain ticker instead of gomega.Eventually so the function returns
// an error instead of panicking through gomega's fail handler.
func (h *Helper) waitForAPIReady(ctx context.Context) error {
	logger.Info("waiting for API to be reachable after rollout")

	ticker := time.NewTicker(apiReadyInterval)
	defer ticker.Stop()
	deadline := time.After(apiReadyTimeout)

	for {
		if _, err := h.Client.ListClusters(ctx); err == nil {
			logger.Info("API is reachable after rollout")
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled waiting for API: %w", ctx.Err())
		case <-deadline:
			return fmt.Errorf("API not reachable after %s", apiReadyTimeout)
		case <-ticker.C:
		}
	}
}

// getAndPatchHelmValues reads helm values for the API release, patches the
// entity with the given kind to set required_adapters, and returns the
// marshaled result.
func getAndPatchHelmValues(ctx context.Context, namespace, kind string, adapters map[string]string) ([]byte, error) {
	getCmd := exec.CommandContext(ctx, "helm", "get", "values", helmReleaseName, // #nosec G204
		"--all",
		"--namespace", namespace,
		"--output", "yaml",
	)
	// Use Output() (stdout only). CombinedOutput() would merge stderr
	// warnings (kubeconfig permissions, deprecation notices) into the
	// YAML and corrupt the parse.
	stdout, err := getCmd.Output()
	if err != nil {
		// On error, capture stderr from the ExitError if available.
		stderr := ""
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		return nil, fmt.Errorf("failed to get helm values: %w (stderr: %s)", err, stderr)
	}

	var vals apiValues
	if err := yaml.Unmarshal(stdout, &vals); err != nil {
		return nil, fmt.Errorf("failed to parse helm values: %w", err)
	}

	if err := patchEntityRequiredAdapters(&vals, kind, adapters); err != nil {
		return nil, err
	}

	patchedBytes, err := yaml.Marshal(vals)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal patched values: %w", err)
	}

	return patchedBytes, nil
}

// patchEntityRequiredAdapters finds the entity with the given kind in
// config.entities and sets its required_adapters to the provided list.
func patchEntityRequiredAdapters(vals *apiValues, kind string, adapters map[string]string) error {
	for i := range vals.Config.Entities {
		if vals.Config.Entities[i].Kind == kind {
			vals.Config.Entities[i].RequiredAdapters = adapters
			return nil
		}
	}
	return fmt.Errorf("entity with kind %q not found in config.entities", kind)
}

// RestoreAPIRequiredAdaptersWithRetry restores the API required adapters with retry logic.
// This is designed for use in DeferCleanup to ensure API config is restored even on transient failures.
func (h *Helper) RestoreAPIRequiredAdaptersWithRetry(ctx context.Context, apiChartPath, namespace string, originalAdapters map[string]string, maxRetries int) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := h.UpgradeAPIRequiredAdapters(ctx, apiChartPath, namespace, originalAdapters)
		if err == nil {
			logger.Info("API config restored successfully", "attempt", attempt)
			return nil
		}
		lastErr = err
		logger.Error("failed to restore API config, retrying",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err)
		if attempt < maxRetries {
			timer := time.NewTimer(retryBackoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("context cancelled during API config restore retry: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}

	names := make([]string, 0, len(originalAdapters))
	for name := range originalAdapters {
		names = append(names, name)
	}
	adapterList := strings.Join(names, ",")
	logger.Error("CRITICAL: failed to restore API config after all retries. Manual fix required",
		"max_retries", maxRetries,
		"error", lastErr,
		"original_adapters", adapterList)

	return fmt.Errorf("failed to restore API config after %d retries: %w", maxRetries, lastErr)
}

// GetAPIRequiredClusterAdapters returns the current map of required cluster adapters from config.
func (h *Helper) GetAPIRequiredClusterAdapters() map[string]string {
	return h.Cfg.Adapters.Cluster
}
