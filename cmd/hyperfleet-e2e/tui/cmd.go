package tui

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	pkgtui "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/tui"
)

var Cmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for running and monitoring E2E tests",
	Long:  "Launch an interactive terminal UI to discover, run, and inspect E2E test results.",
	Args:  cobra.NoArgs,
	Run:   run,
}

func run(_ *cobra.Command, _ []string) {
	// Resolve project root (directory containing the binary's working dir or e2e/ subdir).
	projectDir, err := findProjectRoot()
	if err != nil {
		log.Fatalf("cannot locate project root: %v", err)
	}

	e2eDir := filepath.Join(projectDir, "e2e")
	if _, statErr := os.Stat(e2eDir); os.IsNotExist(statErr) {
		log.Fatalf("e2e directory not found at %s — run from the project root", e2eDir)
	}

	// Discover test suites via AST parsing.
	suites, err := pkgtui.DiscoverSuites(e2eDir)
	if err != nil {
		log.Fatalf("test discovery failed: %v", err)
	}
	if len(suites) == 0 {
		fmt.Fprintln(os.Stderr, "warning: no test suites discovered in", e2eDir)
	}

	// Load execution history.
	store, err := pkgtui.NewHistoryStore(projectDir)
	if err != nil {
		log.Fatalf("failed to open history store: %v", err)
	}
	history, err := store.LoadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load history: %v\n", err)
	}

	// Binary path for running test subprocesses (HYPERFLEET_E2E_BINARY or self).
	binaryPath, err := pkgtui.ResolveRunnerBinaryPath()
	if err != nil {
		log.Fatalf("cannot determine binary path: %v", err)
	}

	model := pkgtui.NewModel(binaryPath, e2eDir, projectDir, suites, history)

	// Pass a pointer so SetProgram updates the same model the Program runs.
	// NewProgram(model) by value left model.program nil, so test status never updated.
	p := tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	model.SetProgram(p)

	if _, runErr := p.Run(); runErr != nil {
		log.Fatalf("tui error: %v", runErr)
	}
}

// findProjectRoot walks up from the current directory looking for a go.mod file.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found in any parent directory")
}
