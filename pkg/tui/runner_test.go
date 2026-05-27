package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRunnerBinaryPathFromEnv(t *testing.T) {
	t.Setenv(RunnerBinaryEnvVar, t.TempDir()+"/hyperfleet-e2e")
	binary := os.Getenv(RunnerBinaryEnvVar)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ResolveRunnerBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("binary path = %q, want %q", got, binary)
	}
}

func TestResolveRunnerBinaryPathDefault(t *testing.T) {
	t.Setenv(RunnerBinaryEnvVar, "")

	got, err := ResolveRunnerBinaryPath()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("binary path = %q, want %q", got, want)
	}
}

func TestResolveRunnerBinaryPathMissing(t *testing.T) {
	t.Setenv(RunnerBinaryEnvVar, filepath.Join(t.TempDir(), "missing"))

	_, err := ResolveRunnerBinaryPath()
	if err == nil {
		t.Fatal("expected error for missing binary path")
	}
}

func TestResolveRunnerBinaryPathDirectory(t *testing.T) {
	t.Setenv(RunnerBinaryEnvVar, t.TempDir())

	_, err := ResolveRunnerBinaryPath()
	if err == nil {
		t.Fatal("expected error when binary path is a directory")
	}
}
