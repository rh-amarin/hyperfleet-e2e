package helper

import (
	"strings"
	"testing"
)

func TestPatchEntityRequiredAdapters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		vals     apiValues
		kind     string
		adapters map[string]string
		wantErr  string
	}{
		{
			name: "patches matching entity",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "Cluster", RequiredAdapters: map[string]string{"old-adapter": "http://old-adapter.ns.svc.cluster.local:8082"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: map[string]string{"new-a": "http://new-a.ns.svc.cluster.local:8082", "new-b": "http://new-b.ns.svc.cluster.local:8082"},
		},
		{
			name: "entity kind not found",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool"},
					},
				},
			},
			kind:     "Cluster",
			adapters: map[string]string{"a": "http://a.ns.svc.cluster.local:8082"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name:     "empty entities",
			vals:     apiValues{},
			kind:     "Cluster",
			adapters: map[string]string{"a": "http://a.ns.svc.cluster.local:8082"},
			wantErr:  `entity with kind "Cluster" not found`,
		},
		{
			name: "multiple entities patches correct one",
			vals: apiValues{
				Config: struct {
					Entities []apiEntity    `yaml:"entities"`
					Rest     map[string]any `yaml:",inline"`
				}{
					Entities: []apiEntity{
						{Kind: "NodePool", RequiredAdapters: map[string]string{"np-adapter": "http://np-adapter.ns.svc.cluster.local:8082"}},
						{Kind: "Cluster", RequiredAdapters: map[string]string{"old": "http://old.ns.svc.cluster.local:8082"}},
					},
				},
			},
			kind:     "Cluster",
			adapters: map[string]string{"new-a": "http://new-a.ns.svc.cluster.local:8082", "new-b": "http://new-b.ns.svc.cluster.local:8082"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := patchEntityRequiredAdapters(&tt.vals, tt.kind, tt.adapters)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify the patched entity and that non-matching entities are unchanged
			found := false
			for _, e := range tt.vals.Config.Entities {
				if e.Kind == tt.kind {
					found = true
					if len(e.RequiredAdapters) != len(tt.adapters) {
						t.Fatalf("patched adapters len = %d, want %d", len(e.RequiredAdapters), len(tt.adapters))
					}
					for name, url := range tt.adapters {
						if e.RequiredAdapters[name] != url {
							t.Errorf("adapter[%s] = %v, want %v", name, e.RequiredAdapters[name], url)
						}
					}
				}
			}
			if !found {
				t.Fatal("patched entity not found after successful patch")
			}

			// Assert non-matching entities were not modified
			for _, e := range tt.vals.Config.Entities {
				if e.Kind == tt.kind {
					continue
				}
				if e.Kind == "NodePool" {
					if len(e.RequiredAdapters) != 1 || e.RequiredAdapters["np-adapter"] != "http://np-adapter.ns.svc.cluster.local:8082" {
						t.Errorf("non-target entity %s was modified: RequiredAdapters = %v", e.Kind, e.RequiredAdapters)
					}
				}
			}
		})
	}
}
