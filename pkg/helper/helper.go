package helper

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// Helper provides utility functions for e2e tests
type Helper struct {
	Cfg           *config.Config
	Client        *client.HyperFleetClient
	K8sClient     *k8sclient.Client
	MaestroClient *maestro.Client
}

// TestDataPath resolves a relative path within the testdata directory
// This ensures testdata paths work correctly whether invoked via go test or the e2e binary
func (h *Helper) TestDataPath(relativePath string) string {
	return filepath.Join(h.Cfg.TestDataDir, relativePath)
}

// GetTestCluster creates a new temporary test cluster
func (h *Helper) GetTestCluster(ctx context.Context, payloadPath string) (string, error) {
	cluster, err := h.Client.CreateClusterFromPayload(ctx, payloadPath)
	if err != nil {
		return "", err
	}
	if cluster == nil {
		return "", fmt.Errorf("CreateClusterFromPayload returned nil")
	}
	if cluster.Id == nil {
		return "", fmt.Errorf("created cluster has no ID")
	}
	return *cluster.Id, nil
}

// CleanupTestCluster deletes the test cluster via the HyperFleet API and waits for hard-delete (404).
// The API DELETE owns the full cleanup lifecycle: adapter finalization, Maestro teardown, namespace deletion.
// Errors from the delete call are logged but do not abort the wait — the cluster may already be deleting.
func (h *Helper) CleanupTestCluster(ctx context.Context, clusterID string) error {
	logger.Info("deleting cluster via API", "cluster_id", clusterID)

	if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
		logger.Info("delete call returned error, proceeding to wait for hard-delete", "cluster_id", clusterID, "error", err)
	}

	pollFn := h.PollClusterHTTPStatus(ctx, clusterID)
	deadline := time.Now().Add(h.Cfg.Timeouts.Cluster.Deleted)
	for time.Now().Before(deadline) {
		status, err := pollFn()
		if err == nil && status == http.StatusNotFound {
			logger.Info("cluster hard-deleted", "cluster_id", clusterID)
			return nil
		}
		time.Sleep(h.Cfg.Polling.Interval)
	}

	return fmt.Errorf("cluster %s not hard-deleted within %s", clusterID, h.Cfg.Timeouts.Cluster.Deleted)
}

// GetMaestroClient returns the Maestro client, initializing it lazily on first access
// This avoids the overhead of K8s service discovery for test suites that don't use Maestro
func (h *Helper) GetMaestroClient() *maestro.Client {
	if h.MaestroClient == nil {
		h.MaestroClient = maestro.NewClient("")
	}
	return h.MaestroClient
}
