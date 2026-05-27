package tui

import (
	"testing"
)

func TestSortEnvVarsByKey(t *testing.T) {
	vars := []EnvVar{
		{Key: "ZEBRA"},
		{Key: "ALPHA"},
		{Key: "MAESTRO_URL"},
	}

	sortEnvVarsByKey(vars)

	want := []string{"ALPHA", "MAESTRO_URL", "ZEBRA"}
	for i, key := range want {
		if vars[i].Key != key {
			t.Fatalf("vars[%d].Key = %q, want %q", i, vars[i].Key, key)
		}
	}
}

func TestNewModelEnvVarsSorted(t *testing.T) {
	m := NewModel("", "", "", nil, nil)

	for i := 1; i < len(m.envVars); i++ {
		if m.envVars[i-1].Key > m.envVars[i].Key {
			t.Fatalf("env vars not sorted: %q before %q", m.envVars[i-1].Key, m.envVars[i].Key)
		}
	}
}
