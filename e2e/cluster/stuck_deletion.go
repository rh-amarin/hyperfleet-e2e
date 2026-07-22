package cluster

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][negative] Stuck Deletion -- Adapter Unable to Finalize Prevents Hard-Delete",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2, labels.Negative),
	func() {
		var (
			h                *helper.Helper
			adapterChartPath string
			apiChartPath     string
			baseDeployOpts   helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupAdapterChart func() error
			var err error
			adapterChartPath, cleanupAdapterChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned adapter Helm chart")
				if err := cleanupAdapterChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup adapter chart: %v\n", err)
				}
			})

			ginkgo.By("Clone API Helm chart repository")
			var cleanupAPIChart func() error
			apiChartPath, cleanupAPIChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "api",
				RepoURL:   h.Cfg.APIDeployment.ChartRepo,
				Ref:       h.Cfg.APIDeployment.ChartRef,
				ChartPath: h.Cfg.APIDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone API Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned API Helm chart")
				if err := cleanupAPIChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup API chart: %v\n", err)
				}
			})

			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace:    h.Cfg.Namespace,
				ChartPath:    adapterChartPath,
				ResourceType: helper.ResourceTypeClusters,
			}
		})

		ginkgo.It("should prevent hard-delete when an adapter cannot finalize",
			func(ctx context.Context) {
				adapterName := "cl-stuck"

				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				ginkgo.By("Deploy dedicated stuck-adapter")
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Uninstall stuck-adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy stuck-adapter")
				ginkgo.GinkgoWriter.Printf("Deployed stuck-adapter: release=%s\n", releaseName)

				ginkgo.By("Upgrade API to add stuck-adapter to required adapters")
				originalAdapters := h.GetAPIRequiredClusterAdapters()
				updatedAdapters := make(map[string]string, len(originalAdapters)+1)
				for k, v := range originalAdapters {
					updatedAdapters[k] = v
				}
				updatedAdapters[adapterName] = fmt.Sprintf("http://%s.%s.svc.cluster.local:8082", adapterName, h.Cfg.Namespace)

				// Register API config restore AFTER adapter cleanup registration (LIFO → executes FIRST)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Restore API required adapters to original config")
					if err := h.RestoreAPIRequiredAdaptersWithRetry(ctx, apiChartPath, h.Cfg.Namespace, originalAdapters, 3); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: %v\n", err)
					}
				})

				err = h.UpgradeAPIRequiredAdapters(ctx, apiChartPath, h.Cfg.Namespace, updatedAdapters)
				Expect(err).NotTo(HaveOccurred(), "failed to upgrade API with stuck-adapter in required adapters")

				deploymentName, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, releaseName)
				Expect(err).NotTo(HaveOccurred(), "failed to find stuck-adapter deployment name")

				ginkgo.By("Create cluster and wait for Reconciled with all adapters including stuck-adapter")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, client.ResourceConditionStatusTrue))

				ginkgo.By("Verify stuck-adapter reported Applied=True")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					var found bool
					for _, s := range statuses.Items {
						if s.Adapter == adapterName {
							found = true
							g.Expect(h.HasAdapterCondition(s.Conditions,
								client.ConditionTypeApplied, client.AdapterConditionStatusTrue)).To(BeTrue(),
								"stuck-adapter should have Applied=True before scale-down")
							break
						}
					}
					g.Expect(found).To(BeTrue(), "stuck-adapter should be present in statuses")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Scale down stuck-adapter to simulate unavailability")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down stuck-adapter")

				ginkgo.By("Soft-delete the cluster")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")

				ginkgo.By("Wait for healthy adapters to report Finalized=True")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					adapterMap := make(map[string]client.AdapterStatus, len(statuses.Items))
					for _, s := range statuses.Items {
						adapterMap[s.Adapter] = s
					}

					for name := range originalAdapters {
						adapter, exists := adapterMap[name]
						g.Expect(exists).To(BeTrue(), "adapter %s should be present", name)
						g.Expect(h.HasAdapterCondition(adapter.Conditions,
							client.ConditionTypeFinalized, client.AdapterConditionStatusTrue)).To(BeTrue(),
							"adapter %s should have Finalized=True", name)
					}
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify cluster remains stuck in soft-deleted state (not hard-deleted)")
				Consistently(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "cluster should still be accessible (not hard-deleted)")
					g.Expect(cl.DeletedTime).NotTo(BeNil(), "cluster should still be soft-deleted")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, client.ResourceConditionStatusFalse)).To(BeTrue(),
						"cluster Reconciled should remain False while stuck-adapter is unavailable")

					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					for _, s := range statuses.Items {
						if s.Adapter == adapterName {
							g.Expect(h.HasAdapterCondition(s.Conditions,
								client.ConditionTypeFinalized, client.AdapterConditionStatusTrue)).To(BeFalse(),
								"stuck-adapter should NOT have Finalized=True while scaled to 0")
							break
						}
					}
				}, h.Cfg.Timeouts.Adapter.Processing/2, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Verified: cluster stuck in soft-deleted state, healthy adapters finalized but stuck-adapter has not\n")

				ginkgo.By("Restore stuck-adapter by scaling up")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 1)
				Expect(err).NotTo(HaveOccurred(), "failed to scale up stuck-adapter")

				ginkgo.By("Verify cluster is hard-deleted after stuck-adapter recovery")
				Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("Verify downstream K8s namespace is cleaned up")
				Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(BeEmpty())

				ginkgo.GinkgoWriter.Printf("Verified: stuck-adapter recovered, cluster hard-deleted\n")
			})
	},
)
