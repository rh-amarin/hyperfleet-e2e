package e2e

import (
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	pkge2e "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/e2e"
)

// TestE2E is the standard Go test entry point used by the ginkgo CLI and go test.
// It bootstraps configuration from environment variables and config file, then hands
// off to Ginkgo. Use the ginkgo CLI for parallel execution:
//
//	ginkgo --procs=4 ./e2e
func TestE2E(t *testing.T) {
	viper.SetEnvPrefix(config.EnvPrefix)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	_ = viper.BindEnv(config.Tests.GinkgoLabelFilter, "GINKGO_LABEL_FILTER")
	_ = viper.BindEnv(config.Tests.GinkgoFocus, "GINKGO_FOCUS")
	_ = viper.BindEnv(config.Tests.GinkgoSkip, "GINKGO_SKIP")

	viper.SetConfigFile("configs/config.yaml")
	viper.SetConfigType("yaml")
	_ = viper.ReadInConfig()

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	pkge2e.SetSuiteConfig(cfg)

	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()
	if v := viper.GetString(config.Tests.GinkgoLabelFilter); v != "" {
		suiteConfig.LabelFilter = v
	}
	if v := viper.GetString(config.Tests.GinkgoFocus); v != "" {
		suiteConfig.FocusStrings = append(suiteConfig.FocusStrings, v)
	}
	if v := viper.GetString(config.Tests.GinkgoSkip); v != "" {
		suiteConfig.SkipStrings = append(suiteConfig.SkipStrings, v)
	}

	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "HyperFleet E2E Suite", suiteConfig, reporterConfig)
}
