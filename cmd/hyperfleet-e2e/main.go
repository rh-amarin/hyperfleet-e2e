package main

import (
	"log"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/common"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/test"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/tui"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

var root = &cobra.Command{
	Use:           "hyperfleet-e2e",
	Short:         "HyperFleet end-to-end testing tool",
	Long:          "Command line tool for running HyperFleet e2e tests.",
	SilenceErrors: true,
	SilenceUsage:  true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Set config file in common package for access by subcommands
		common.ConfigFile = configFile
	},
}

func init() {
	pfs := root.PersistentFlags()
	pfs.StringVar(&configFile, "config", "", "config file path")
	pfs.StringVar(&apiURL, "api-url", "", "HyperFleet API URL")
	pfs.StringVar(&logLevel, "log-level", config.DefaultLogLevel, "Log level (debug, info, warn, error)")
	pfs.StringVar(&logFormat, "log-format", config.DefaultLogFormat, "Log format (text, json)")
	pfs.StringVar(&logOutput, "log-output", config.DefaultLogOutput, "Log output (stdout, stderr)")

	// Flags are bound in subcommand run() after config loading (osde2e pattern)

	root.AddCommand(test.Cmd)
	root.AddCommand(tui.Cmd)
}

var (
	configFile string
	apiURL     string
	logLevel   string
	logFormat  string
	logOutput  string
)

func main() {
	if err := root.Execute(); err != nil {
		log.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}
