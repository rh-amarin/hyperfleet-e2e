package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper/helm"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

var (
	AppliedManifestWorksGVR = &schema.GroupVersionResource{
		Group:    "work.open-cluster-management.io",
		Version:  "v1",
		Resource: "appliedmanifestworks",
	}
)

type CleanupHelper struct {
	cfg                      *config.Config
	k8sClient                *kubernetes.Client
	dynamicClient            *kubernetes.DynamicClient
	labelSelectorListOptions metav1.ListOptions
	adapterDeploymentList    *AdapterDeploymentList
}

// NewCleanupHelper creates a new CleanupHelper
// It creates a new helper instance and uses it to cleanup resources
func NewCleanupHelper() (*CleanupHelper, error) {
	cfg := GetSuiteConfig()
	if cfg == nil {
		return nil, fmt.Errorf("config must be set for resource cleanup tracking")
	}
	k8sClient, err := kubernetes.NewClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	adapterDeploymentList := GetAdapterDeploymentList()
	if adapterDeploymentList == nil {
		return nil, fmt.Errorf("adapter deployment list must be set for resource cleanup tracking")
	}
	// RUN_ID is set when tests are executed, forms the label: e2e.hyperfleet.io/run-id=<run_id>
	// Label applied to adapters and resources created during test suite execution
	labelSelector := fmt.Sprintf("e2e.hyperfleet.io/run-id=%s", cfg.RunID)
	dynamicClient, err := kubernetes.NewDynamicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}
	labelSelectorListOptions := metav1.ListOptions{
		LabelSelector: labelSelector,
	}

	return &CleanupHelper{k8sClient: k8sClient, dynamicClient: dynamicClient, labelSelectorListOptions: labelSelectorListOptions, cfg: cfg, adapterDeploymentList: adapterDeploymentList}, nil
}

// CleanupResources is the entry point for the end-of-suite cleanup mechanism that does a final sweep
// Using the label selector = e2e.hyperfleet.io/run-id=<run-id> that is set when the tests are initiated
func CleanupResources() {
	c, err := NewCleanupHelper()
	if err != nil {
		logger.Error("failed to create cleanup helper", "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Step 1: Remove all helm releases installed with the given label selector
	helmClient := helm.NewClient(c.cfg.Namespace)
	releases, err := helmClient.ListReleasesBySelector(c.labelSelectorListOptions.LabelSelector)
	if err != nil {
		// Failed to list releases, so skipping uninstall
		logger.Error("failed to list helm releases", "error", err)
		// Still proceeding with cleanup
	} else {
		logger.Info("found helm releases", "count", len(releases))
		for _, release := range releases {
			err := helmClient.UninstallRelease(ctx, release, c.cfg.Namespace)
			if err != nil {
				logger.Error("failed to uninstall helm release", "name", release, "error", err)
				continue
			}
			logger.Info("uninstalled helm chart", "name", release)
		}
	}

	// Step 2: Sweep resources that contain the given label selector
	if err := c.SweepLabeledResources(ctx); err != nil {
		logger.Error("failed to cleanup test resources", "error", err)
	}

	logger.Info("test resources cleaned up")
}


// SweepLabeledResources iterates through labeled resources and deletes any orphaned resources
func (c *CleanupHelper) SweepLabeledResources(ctx context.Context) error {
	logger.Info("Starting robust test cleanup with label selector:", "labelSelector", c.labelSelectorListOptions.LabelSelector)

	// Phase 1: Remove AppliedManifestWork finalizers to break controller dependencies
	logger.Info("Phase 1: Removing finalizers from AppliedManifestWorks")
	c.removeAppliedManifestWorkFinalizers(ctx)

	// Phase 2: Delete resources (Background propagation - don't wait for cascade)
	logger.Info("Phase 2: Deleting resources")
	if err := c.deleteJobs(ctx); err != nil {
		logger.Error("failed to delete jobs", "error", err)
	}
	if err := c.deleteDeployments(ctx); err != nil {
		logger.Error("failed to delete deployments", "error", err)
	}
	if err := c.deleteConfigMaps(ctx); err != nil {
		logger.Error("failed to delete configmaps", "error", err)
	}
	if err := c.deleteAppliedManifestWorksForce(ctx); err != nil {
		logger.Error("failed to delete applied manifest works", "error", err)
	}
	if err := c.deleteNamespacesForce(ctx); err != nil {
		logger.Error("failed to delete namespaces", "error", err)
	}

	// Phase 3: Wait for deletion to complete (poll until resources are gone or timeout)
	logger.Info("Phase 3: Waiting for deletion to complete (polling for up to 3 minutes)")
	err := wait.PollUntilContextTimeout(ctx, 1*time.Minute, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		remaining, err := c.countRemainingResources(ctx)
		if err != nil {
			logger.Error("failed to count remaining resources", "error", err)
			return false, nil
		}
		if remaining == 0 {
			logger.Info("all resources deleted successfully")
			return true, nil
		}
		logger.Info("resources still being deleted", "remaining", remaining)
		return false, nil
	})

	if err != nil {
		return fmt.Errorf("not all resources were deleted in time: %w", err)
	}

	logger.Info("cleanup completed successfully with all resources deleted")
	return nil
}

// removeAllFinalizers strips finalizers from AppliedManifestWorks
// These are the resources with custom finalizers that block deletion
func (c *CleanupHelper) removeAppliedManifestWorkFinalizers(ctx context.Context) {
	amws, err := c.dynamicClient.Resource(*AppliedManifestWorksGVR).List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list AppliedManifestWorks for finalizer removal", "error", err)
		return
	}
	for _, amw := range amws.Items {
		if len(amw.GetFinalizers()) > 0 {
			name := amw.GetName()
			logger.Info("Removing finalizers from AppliedManifestWork", "name", name, "finalizers", amw.GetFinalizers())

			// Patch to remove finalizers
			patch := []byte(`{"metadata":{"finalizers":null}}`)
			_, err := c.dynamicClient.Resource(*AppliedManifestWorksGVR).Patch(
				ctx,
				name,
				types.MergePatchType,
				patch,
				metav1.PatchOptions{},
			)
			if err != nil {
				logger.Error("failed to remove finalizers from AppliedManifestWork", "name", name, "error", err)
			}
		}
	}
}

// deleteAppliedManifestWorksForce deletes AMWs without relying on propagation
func (c *CleanupHelper) deleteAppliedManifestWorksForce(ctx context.Context) error {
	appliedManifestWorks, err := c.dynamicClient.Resource(*AppliedManifestWorksGVR).List(ctx, c.labelSelectorListOptions)
	if err != nil {
		return fmt.Errorf("failed to list AppliedManifestWorks: %w", err)
	}

	// Use Background propagation - we already removed finalizers
	propagationPolicy := metav1.DeletePropagationBackground
	zeroInt64 := int64(0)
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy:  &propagationPolicy,
		GracePeriodSeconds: &zeroInt64,
	}

	for _, appliedManifestWork := range appliedManifestWorks.Items {
		appliedManifestWorkName := appliedManifestWork.GetName()
		logger.Warn("Deleting AppliedManifestWork", "name", appliedManifestWorkName)
		if err := c.dynamicClient.Resource(*AppliedManifestWorksGVR).Delete(ctx, appliedManifestWorkName, deleteOptions); err != nil {
			logger.Error("failed to delete AppliedManifestWork", "name", appliedManifestWorkName, "error", err)
		}
	}
	return nil
}

// countRemainingResources returns the count of resources still present
// If any error occurs, the count that was obtained is returned and an error is returned
func (c *CleanupHelper) countRemainingResources(ctx context.Context) (int, error) {
	count := 0
	errorList := []string{}
	// Count AppliedManifestWorks
	amws, err := c.dynamicClient.Resource(*AppliedManifestWorksGVR).List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list AppliedManifestWorks", "error", err)
		errorList = append(errorList, "AppliedManifestWorks")
	} else {
		count += len(amws.Items)
	}

	// Count Namespaces
	namespaces, err := c.k8sClient.CoreV1().Namespaces().List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list Namespaces", "error", err)
		errorList = append(errorList, "Namespaces")
	} else {
		count += len(namespaces.Items)
	}

	// Count Jobs
	jobs, err := c.k8sClient.BatchV1().Jobs("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list Jobs", "error", err)
		errorList = append(errorList, "Jobs")
	} else {
		count += len(jobs.Items)
	}

	// Count Deployments
	deployments, err := c.k8sClient.AppsV1().Deployments("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list Deployments", "error", err)
		errorList = append(errorList, "Deployments")
	} else {
		count += len(deployments.Items)
	}

	// Count ConfigMaps
	configMaps, err := c.k8sClient.CoreV1().ConfigMaps("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		logger.Error("failed to list ConfigMaps", "error", err)
		errorList = append(errorList, "ConfigMaps")
	} else {
		count += len(configMaps.Items)
	}

	if len(errorList) > 0 {
		// Return the count that was obtained and an error message of the resources that failed to list
		return count, fmt.Errorf("failed to list some resources: %s", strings.Join(errorList, ", "))
	}

	return count, nil
}

// deleteJobs deletes all jobs matching the label selector
func (c *CleanupHelper) deleteJobs(ctx context.Context) error {
	jobs, err := c.k8sClient.BatchV1().Jobs("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		return fmt.Errorf("failed to list Jobs: %w", err)
	}
	for _, job := range jobs.Items {
		logger.Warn("deleting Job", "name", job.Name, "namespace", job.Namespace)
		if err := c.k8sClient.BatchV1().Jobs(job.Namespace).Delete(ctx, job.Name, metav1.DeleteOptions{}); err != nil {
			logger.Error("failed to delete Job", "name", job.Name, "namespace", job.Namespace, "error", err)
		}
	}
	return nil
}

func (c *CleanupHelper) deleteDeployments(ctx context.Context) error {
	deployments, err := c.k8sClient.AppsV1().Deployments("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		return fmt.Errorf("failed to list Deployments: %w", err)
	}
	for _, deployment := range deployments.Items {
		logger.Warn("deleting Deployment", "name", deployment.Name, "namespace", deployment.Namespace)
		if err := c.k8sClient.AppsV1().Deployments(deployment.Namespace).Delete(ctx, deployment.Name, metav1.DeleteOptions{}); err != nil {
			logger.Error("failed to delete Deployment", "name", deployment.Name, "namespace", deployment.Namespace, "error", err)
		}
	}
	return nil
}

func (c *CleanupHelper) deleteConfigMaps(ctx context.Context) error {
	configMaps, err := c.k8sClient.CoreV1().ConfigMaps("").List(ctx, c.labelSelectorListOptions)
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps: %w", err)
	}
	for _, configMap := range configMaps.Items {
		logger.Warn("deleting ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace)
		if err := c.k8sClient.CoreV1().ConfigMaps(configMap.Namespace).Delete(ctx, configMap.Name, metav1.DeleteOptions{}); err != nil {
			logger.Error("failed to delete ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace, "error", err)
		}
	}
	return nil
}

func (c *CleanupHelper) deleteNamespacesForce(ctx context.Context) error {
	namespaces, err := c.k8sClient.CoreV1().Namespaces().List(ctx, c.labelSelectorListOptions)
	if err != nil {
		return fmt.Errorf("failed to list Namespaces: %w", err)
	}

	propagationPolicy := metav1.DeletePropagationBackground
	zeroInt64 := int64(0)
	deleteOptions := metav1.DeleteOptions{
		PropagationPolicy:  &propagationPolicy,
		GracePeriodSeconds: &zeroInt64,
	}
	for _, namespace := range namespaces.Items {
		logger.Warn("deleting Namespace", "name", namespace.Name)
		if err := c.k8sClient.CoreV1().Namespaces().Delete(ctx, namespace.Name, deleteOptions); err != nil {
			logger.Error("failed to delete Namespace", "name", namespace.Name, "error", err)
		}
	}
	return nil
}
