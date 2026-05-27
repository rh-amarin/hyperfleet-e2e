package tui

import (
	"testing"
)

func TestResetParallelInputDefaultUsesMatchingSuiteCount(t *testing.T) {
	m := Model{
		suites: []TestSuite{
			{Name: "a", Labels: []string{"tier1"}},
			{Name: "b", Labels: []string{"tier1"}},
			{Name: "c", Labels: []string{"tier2"}},
		},
		newExec: newExecModel{
			filterIdx: 1, // tier1
		},
	}

	m.resetParallelInputDefault()

	if got := m.newExec.parallelInput.Value(); got != "2" {
		t.Fatalf("parallel default = %q, want %q", got, "2")
	}
}

func TestResetParallelInputDefaultMinimumOne(t *testing.T) {
	m := Model{
		newExec: newExecModel{
			filterIdx: 4, // custom
		},
	}

	m.resetParallelInputDefault()

	if got := m.newExec.parallelInput.Value(); got != "1" {
		t.Fatalf("parallel default = %q, want %q", got, "1")
	}
}
