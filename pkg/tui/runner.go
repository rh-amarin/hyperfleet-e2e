package tui

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

// RunnerBinaryEnvVar is the environment variable that overrides the binary path
// used to spawn test subprocesses from the TUI. When unset, os.Executable() is used.
var RunnerBinaryEnvVar = config.EnvVar("E2E_BINARY")

// ResolveRunnerBinaryPath returns the hyperfleet-e2e binary used to spawn test subprocesses.
func ResolveRunnerBinaryPath() (string, error) {
	if p := os.Getenv(RunnerBinaryEnvVar); p != "" {
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("%s=%q: %w", RunnerBinaryEnvVar, p, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("%s=%q is a directory", RunnerBinaryEnvVar, p)
		}
		return p, nil
	}
	return os.Executable()
}

// launchTestRunner starts goroutines for the given test indices on an execution.
// Caller must set e.Status and reset test rows before calling.
func (m *Model) launchTestRunner(e *Execution, indices []int, parallelism int, labelFilter string) {
	if parallelism < 1 {
		parallelism = 1
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.runCancel = cancel
	m.runExecID = e.ID

	p := m.program
	binaryPath := m.binaryPath
	envVars := make([]EnvVar, len(m.envVars))
	copy(envVars, m.envVars)

	go func(runCtx context.Context) {
		sem := make(chan struct{}, parallelism)
		var wg sync.WaitGroup

		for _, idx := range indices {
			if idx < 0 || idx >= len(e.Tests) {
				continue
			}
			test := e.Tests[idx]
			test.Suite = m.healSuiteFocus(test.Suite, e)
			wg.Add(1)
			suite := test.Suite
			logFile := test.LogFile
			testIdx := idx

			go func() {
				defer wg.Done()

				select {
				case <-runCtx.Done():
					if p != nil {
						p.Send(testDoneMsg{execID: e.ID, testIdx: testIdx, endAt: time.Now(), stopped: true})
					}
					return
				case sem <- struct{}{}:
				}
				defer func() { <-sem }()

				startAt := time.Now()
				if p != nil {
					p.Send(testStartedMsg{execID: e.ID, testIdx: testIdx, startAt: startAt})
				}

				passed, stopped := runTestProcess(runCtx, binaryPath, suite, logFile, labelFilter, envVars)
				endAt := time.Now()

				if p != nil {
					p.Send(testDoneMsg{execID: e.ID, testIdx: testIdx, endAt: endAt, passed: passed, stopped: stopped})
				}
			}()
		}

		wg.Wait()
		cancelled := runCtx.Err() != nil
		if p != nil {
			p.Send(execDoneMsg{execID: e.ID, endAt: time.Now(), cancelled: cancelled})
		}
	}(ctx)
}
