package cluster

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][negative] Cluster Can Reflect Adapter Failure in Top-Level Status",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var (
			h              *helper.Helper
			chartPath      string
			baseDeployOpts helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Clone adapter Helm chart repository (shared across all tests in this Describe)
			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupChart func() error
			var err error
			chartPath, cleanupChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")
			ginkgo.GinkgoWriter.Printf("Cloned adapter chart to: %s\n", chartPath)

			// Ensure chart cleanup after test
			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned Helm chart")
				if err := cleanupChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup chart: %v\n", err)
				}
			})

			// Set up base deployment options with common fields
			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace:    h.Cfg.Namespace,
				ChartPath:    chartPath,
				ResourceType: helper.ResourceTypeClusters,
			}
		})

		ginkgo.It("should not block cluster reconciliation when non-required adapter has param extraction failure",
			func(ctx context.Context) {
				adapterName := "cl-param-error"

				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the adapter with an invalid API URL in params
				ginkgo.By("Deploy dedicated adapter with invalid API URL in params")

				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				// Ensure adapter cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Uninstall cl-param-error adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					} else {
						ginkgo.GinkgoWriter.Printf("Successfully uninstalled adapter: %s\n", releaseName)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy cl-param-error adapter")
				ginkgo.GinkgoWriter.Printf("Deployed cl-param-error adapter: release=%s\n", releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Submit an API request to create a Cluster resource")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Ensure cluster cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				// Step 3: Verify cluster reconciles normally despite non-required adapter failure
				ginkgo.By("Verify cluster reconciles normally despite non-required adapter param extraction failure")
				// cl-param-error is a non-required adapter with required:true on a param
				// that points to an invalid URL. Param extraction fails → early return → no status reported.
				// Per ADR-0008, aggregated conditions evaluate only required adapters,
				// so this adapter's failure does NOT prevent reconciliation.
				Eventually(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, client.ResourceConditionStatusTrue)).To(BeTrue(),
						"cluster Reconciled should become True despite non-required adapter failure")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeLastKnownReconciled, client.ResourceConditionStatusTrue)).To(BeTrue(),
						"cluster LastKnownReconciled should become True despite non-required adapter failure")
				}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())

				// Step 4: Verify adapter status is absent — required param extraction failure
				// causes early return in executor (before post_actions), so no status is reported.
				ginkgo.By("Verify adapter status is absent (param extraction failure prevents status reporting)")
				statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

				var adapterStatus *client.AdapterStatus
				for i, status := range statuses.Items {
					if status.Adapter == adapterName {
						adapterStatus = &statuses.Items[i]
						break
					}
				}
				Expect(adapterStatus).To(BeNil(),
					"adapter with required param extraction failure should not report status (post_actions never execute)")

				ginkgo.GinkgoWriter.Printf("Verified: adapter status absent (required param failure), cluster reconciled normally\n")
			})
	},
)
