package helper

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// hashSuffixPattern matches the deterministic hash appended on truncation:
// dash followed by exactly 8 lowercase hex chars at end of string.
var hashSuffixPattern = regexp.MustCompile(`-[0-9a-f]{8}$`)

// TestGenerateAdapterReleaseName covers the main behavioral properties:
// - short names pass through unchanged
// - names exactly at the limit pass through unchanged
// - over-limit names get truncated AND get a hash suffix
// - output never exceeds the max length
func TestGenerateAdapterReleaseName(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		adapterName  string
		wantExact    string // when set, assert exact equality
		wantHashed   bool   // expect hash suffix at the end
	}{
		{
			name:         "short name, no truncation",
			resourceType: "clusters",
			adapterName:  "validation",
			wantExact:    "adapter-clusters-validation",
			wantHashed:   false,
		},
		{
			name:         "typical adapter name",
			resourceType: "nodepools",
			adapterName:  "maestro",
			wantExact:    "adapter-nodepools-maestro",
			wantHashed:   false,
		},
		{
			name:         "exactly at limit, no truncation",
			resourceType: "clusters",
			adapterName:  strings.Repeat("x", maxReleaseNameLength-len("adapter-clusters-")),
			wantExact:    "adapter-clusters-" + strings.Repeat("x", maxReleaseNameLength-len("adapter-clusters-")),
			wantHashed:   false,
		},
		{
			name:         "over limit, truncated with hash",
			resourceType: "clusters",
			adapterName:  "my-very-long-adapter-name-that-exceeds-the-limit",
			wantHashed:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GenerateAdapterReleaseName(tt.resourceType, tt.adapterName)

			if len(got) > maxReleaseNameLength {
				t.Errorf("length %d exceeds max %d: %q", len(got), maxReleaseNameLength, got)
			}

			wantPrefix := fmt.Sprintf("adapter-%s-", tt.resourceType)
			if !strings.HasPrefix(got, wantPrefix) {
				t.Errorf("missing prefix %q in %q", wantPrefix, got)
			}

			hasHash := hashSuffixPattern.MatchString(got)
			if hasHash != tt.wantHashed {
				t.Errorf("hash suffix presence: got %v, want %v (name: %q)", hasHash, tt.wantHashed, got)
			}

			if tt.wantExact != "" && got != tt.wantExact {
				t.Errorf("got %q, want %q", got, tt.wantExact)
			}
		})
	}
}

// TestGenerateAdapterReleaseName_Deterministic asserts that release names are
// fully deterministic: identical inputs must always produce identical outputs.
// This is a hard requirement — uninstall flows locate releases by exact name,
// and any randomness here would cause cleanup to leak orphan Helm releases.
func TestGenerateAdapterReleaseName_Deterministic(t *testing.T) {
	cases := []struct {
		resourceType string
		adapterName  string
	}{
		{"clusters", "foo"},
		{"nodepools", "maestro"},
		{"clusters", "my-very-long-adapter-name-that-exceeds-the-limit"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%s", tc.resourceType, tc.adapterName), func(t *testing.T) {
			a := GenerateAdapterReleaseName(tc.resourceType, tc.adapterName)
			b := GenerateAdapterReleaseName(tc.resourceType, tc.adapterName)
			if a != b {
				t.Errorf("non-deterministic output: %q != %q", a, b)
			}
		})
	}
}

func newHelperWithService(ns string, svc *corev1.Service) *Helper {
	var objs []k8sruntime.Object
	if svc != nil {
		objs = append(objs, svc)
	}
	return &Helper{
		Cfg:       &config.Config{Namespace: ns},
		K8sClient: &k8sclient.Client{Interface: fake.NewClientset(objs...)},
	}
}

func TestResolveInternalAPIURL(t *testing.T) {
	const ns = "hyperfleet-system"

	svcWithPort := func(port int32) *corev1.Service {
		return &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "hyperfleet-api", Namespace: ns},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{Port: port}},
			},
		}
	}

	tests := []struct {
		name       string
		svc        *corev1.Service
		wantURL    string
		wantErrMsg string
	}{
		{
			name:    "service found with port",
			svc:     svcWithPort(8000),
			wantURL: fmt.Sprintf("http://hyperfleet-api.%s.svc.cluster.local:8000", ns),
		},
		{
			name:       "service not found",
			svc:        nil,
			wantErrMsg: `failed to get hyperfleet-api service in namespace "hyperfleet-system"`,
		},
		{
			name: "service found but no ports",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "hyperfleet-api", Namespace: ns},
				Spec:       corev1.ServiceSpec{},
			},
			wantErrMsg: "hyperfleet-api service has no ports",
		},
		{
			// A hyperfleet-api service in a different namespace must not be found
			// when h.Cfg.Namespace is set — Get is scoped to the configured namespace.
			name: "service in wrong namespace is not found",
			svc: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "hyperfleet-api", Namespace: "other-ns"},
				Spec:       corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: 8000}}},
			},
			wantErrMsg: `failed to get hyperfleet-api service in namespace "hyperfleet-system"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHelperWithService(ns, tt.svc)
			got, err := h.resolveInternalAPIURL(context.Background())

			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				if !strings.Contains(err.Error(), tt.wantErrMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("got %q, want %q", got, tt.wantURL)
			}
		})
	}
}

// TestGenerateAdapterReleaseName_LongNameCollision asserts that two distinct
// long names sharing a long common prefix produce distinct release names.
// The hash suffix is what guarantees uniqueness once the base is truncated.
func TestGenerateAdapterReleaseName_LongNameCollision(t *testing.T) {
	a := GenerateAdapterReleaseName("clusters", "adapter-alpha-very-long-name-here")
	b := GenerateAdapterReleaseName("clusters", "adapter-bravo-very-long-name-here")

	if a == b {
		t.Errorf("different inputs produced same release name: %q", a)
	}
}

func TestAdapterHelmSetArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		tokenRequestSA string // ServiceAccountName - non-empty enables JWT
		runID          string
		setValues      map[string]string
		wantContains   []string // substrings that must appear in joined args
		wantAbsent     []string // substrings that must NOT appear
	}{
		{
			name:           "includes auth flag when JWT is enabled",
			tokenRequestSA: "hyperfleet-e2e-sa",
			wantContains:   []string{"adapterConfig.hyperfleetApi.auth.enabled=true"},
		},
		{
			name:       "omits auth flag when JWT is disabled",
			wantAbsent: []string{"adapterConfig.hyperfleetApi.auth.enabled"},
		},
		{
			name:         "includes fullnameOverride",
			wantContains: []string{"fullnameOverride=test-release"},
		},
		{
			name:         "includes run-id label when set",
			runID:        "abc-123",
			wantContains: []string{"e2e.hyperfleet.io/run-id=abc-123"},
		},
		{
			name:       "omits run-id label when empty",
			wantAbsent: []string{"e2e.hyperfleet.io/run-id"},
		},
		{
			name:         "includes custom set values",
			setValues:    map[string]string{"image.tag": "latest"},
			wantContains: []string{"image.tag=latest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{RunID: tt.runID}
			cfg.Identity.TokenRequest.ServiceAccountName = tt.tokenRequestSA

			h := &Helper{Cfg: cfg}
			opts := AdapterDeploymentOptions{SetValues: tt.setValues}
			args := h.adapterHelmSetArgs("test-release", opts)
			joined := strings.Join(args, " ")

			for _, want := range tt.wantContains {
				if !strings.Contains(joined, want) {
					t.Errorf("expected args to contain %q, got: %v", want, args)
				}
			}
			for _, absent := range tt.wantAbsent {
				if strings.Contains(joined, absent) {
					t.Errorf("expected args NOT to contain %q, got: %v", absent, args)
				}
			}
		})
	}
}
