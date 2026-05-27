package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// RunStatus represents the lifecycle state of a test or execution.
type RunStatus string

const (
	StatusPending   RunStatus = "pending"
	StatusRunning   RunStatus = "running"
	StatusPassed    RunStatus = "passed"
	StatusFailed    RunStatus = "failed"
	StatusStopped   RunStatus = "stopped"
	StatusCancelled RunStatus = "cancelled"
)

// TestRun holds the state of a single test suite execution.
type TestRun struct {
	Suite    TestSuite
	Status   RunStatus
	StartAt  time.Time
	EndAt    time.Time
	LogFile  string
	LogLines []string // in-memory tail, not persisted
}

// Duration returns how long the test ran (or is running).
func (t *TestRun) Duration() time.Duration {
	if t.StartAt.IsZero() {
		return 0
	}
	end := t.EndAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(t.StartAt)
}

// ExecFilter describes how an execution was configured.
type ExecFilter struct {
	LabelFilter string `json:"label_filter"` // "tier0", "tier1", "tier2", or ""
	Focus       string `json:"focus"`        // custom --focus regex
	Parallelism int    `json:"parallelism"`
}

// Execution represents one run of a set of test suites.
type Execution struct {
	ID        string     `json:"id"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   time.Time  `json:"ended_at,omitempty"`
	Filter    ExecFilter `json:"filter"`
	Tests     []*TestRun `json:"-"` // loaded separately
	Status    RunStatus  `json:"status"`
	LogDir    string     `json:"log_dir"`
}

// Duration returns total wall time.
func (e *Execution) Duration() time.Duration {
	if e.StartedAt.IsZero() {
		return 0
	}
	end := e.EndedAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(e.StartedAt)
}

// FilterLabel returns a human-readable string for the execution filter.
func (e *Execution) FilterLabel() string {
	if e.Filter.Focus != "" {
		return "focus:" + e.Filter.Focus
	}
	if e.Filter.LabelFilter != "" {
		return e.Filter.LabelFilter
	}
	return "all"
}

// testRunMeta is the persisted form of a TestRun.
type testRunMeta struct {
	SuiteName   string    `json:"suite_name"`
	SuiteFile   string    `json:"suite_file"`
	SuiteLabels []string  `json:"suite_labels"`
	SuiteFocus  string    `json:"suite_focus,omitempty"`
	Status      RunStatus `json:"status"`
	StartAt     time.Time `json:"start_at"`
	EndAt       time.Time `json:"end_at"`
	LogFile     string    `json:"log_file"`
}

// HistoryStore manages execution persistence under .hyperfleet/.
type HistoryStore struct {
	dir string // project root / .hyperfleet
}

// NewHistoryStore creates a store rooted at projectDir/.hyperfleet.
func NewHistoryStore(projectDir string) (*HistoryStore, error) {
	dir := filepath.Join(projectDir, ".hyperfleet")
	if err := os.MkdirAll(filepath.Join(dir, "executions"), 0o750); err != nil {
		return nil, err
	}
	return &HistoryStore{dir: dir}, nil
}

// Save persists an execution record.
func (h *HistoryStore) Save(e *Execution) error {
	execDir := filepath.Join(h.dir, "executions", e.ID)
	if err := os.MkdirAll(execDir, 0o750); err != nil {
		return err
	}

	// Build test run metas
	var metas []testRunMeta
	for _, t := range e.Tests {
		metas = append(metas, testRunMeta{
			SuiteName:   t.Suite.Name,
			SuiteFile:   t.Suite.File,
			SuiteLabels: t.Suite.Labels,
			SuiteFocus:  t.Suite.Focus,
			Status:      t.Status,
			StartAt:     t.StartAt,
			EndAt:       t.EndAt,
			LogFile:     t.LogFile,
		})
	}

	type execRecord struct {
		Execution
		TestRuns []testRunMeta `json:"test_runs"`
	}

	rec := execRecord{Execution: *e, TestRuns: metas}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(execDir, "meta.json"), data, 0o640)
}

// LoadAll loads all executions from disk, newest first.
func (h *HistoryStore) LoadAll() ([]*Execution, error) {
	execsDir := filepath.Join(h.dir, "executions")
	entries, err := os.ReadDir(execsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var execs []*Execution
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		e, loadErr := h.loadOne(filepath.Join(execsDir, entry.Name()))
		if loadErr != nil {
			continue
		}
		// Runs left as "running" on disk were interrupted (e.g. TUI exited).
		if e.Status == StatusRunning {
			e.Status = StatusCancelled
			for _, t := range e.Tests {
				if t.Status == StatusPending || t.Status == StatusRunning {
					t.Status = StatusStopped
				}
			}
		}
		execs = append(execs, e)
	}

	sort.Slice(execs, func(i, j int) bool {
		return execs[i].StartedAt.After(execs[j].StartedAt)
	})
	return execs, nil
}

func (h *HistoryStore) loadOne(execDir string) (*Execution, error) {
	data, err := os.ReadFile(filepath.Join(execDir, "meta.json"))
	if err != nil {
		return nil, err
	}

	type execRecord struct {
		Execution
		TestRuns []testRunMeta `json:"test_runs"`
	}

	var rec execRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, err
	}

	e := &rec.Execution
	for _, m := range rec.TestRuns {
		e.Tests = append(e.Tests, &TestRun{
			Suite: TestSuite{
				Name:   m.SuiteName,
				File:   m.SuiteFile,
				Labels: m.SuiteLabels,
				Focus:  m.SuiteFocus,
			},
			Status:  m.Status,
			StartAt: m.StartAt,
			EndAt:   m.EndAt,
			LogFile: m.LogFile,
		})
	}
	return e, nil
}

// LogDir returns the directory where logs for an execution are stored.
func (h *HistoryStore) LogDir(execID string) string {
	return filepath.Join(h.dir, "executions", execID)
}
