package helper

import (
	"fmt"
	"strings"

	"github.com/onsi/gomega/types"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
)

// HaveResourceCondition matches a *Resource that has the specified condition type and status.
func HaveResourceCondition(condType string, status client.ResourceConditionStatus) types.GomegaMatcher {
	return &resourceConditionMatcher{condType: condType, status: status}
}

type resourceConditionMatcher struct {
	condType string
	status   client.ResourceConditionStatus
	actual   string
}

func (m *resourceConditionMatcher) Match(actual any) (bool, error) {
	conditions, err := extractResourceConditions(actual)
	if err != nil {
		return false, err
	}
	if conditions == nil {
		m.actual = "<nil conditions>"
		return false, nil
	}
	for _, c := range conditions {
		if c.Type == m.condType && c.Status == m.status {
			return true, nil
		}
	}
	m.actual = formatResourceConditions(conditions)
	return false, nil
}

func (m *resourceConditionMatcher) FailureMessage(_ any) string {
	return fmt.Sprintf("expected condition %s=%s but got: %s", m.condType, m.status, m.actual)
}

func (m *resourceConditionMatcher) NegatedFailureMessage(_ any) string {
	return fmt.Sprintf("expected NOT to have condition %s=%s", m.condType, m.status)
}

// HaveAllAdaptersWithCondition matches an *AdapterStatusList where every required
// adapter has the specified condition type and status. requiredAdapters is the
// name→URL map from AdaptersConfig; only the keys (names) are used.
func HaveAllAdaptersWithCondition(requiredAdapters map[string]string, condType string, status client.AdapterConditionStatus) types.GomegaMatcher {
	return &allAdaptersConditionMatcher{
		adapters: requiredAdapters,
		condType: condType,
		status:   status,
	}
}

type allAdaptersConditionMatcher struct {
	adapters map[string]string
	condType string
	status   client.AdapterConditionStatus
	missing  []string
}

func (m *allAdaptersConditionMatcher) Match(actual any) (bool, error) {
	list, ok := actual.(*client.AdapterStatusList)
	if !ok {
		return false, fmt.Errorf("HaveAllAdaptersWithCondition expects *AdapterStatusList, got %T", actual)
	}
	if list == nil {
		return false, fmt.Errorf("HaveAllAdaptersWithCondition expects non-nil *AdapterStatusList")
	}

	m.missing = nil
	adapterMap := make(map[string]client.AdapterStatus, len(list.Items))
	for _, s := range list.Items {
		adapterMap[s.Adapter] = s
	}

	for name := range m.adapters {
		adapter, exists := adapterMap[name]
		if !exists {
			m.missing = append(m.missing, name+" (not found)")
			continue
		}
		if !hasAdapterCond(adapter.Conditions, m.condType, m.status) {
			m.missing = append(m.missing, name)
		}
	}
	return len(m.missing) == 0, nil
}

func (m *allAdaptersConditionMatcher) FailureMessage(_ any) string {
	return fmt.Sprintf("adapters missing %s=%s: %s", m.condType, m.status, strings.Join(m.missing, ", "))
}

func (m *allAdaptersConditionMatcher) NegatedFailureMessage(_ any) string {
	return fmt.Sprintf("expected some adapters NOT to have %s=%s", m.condType, m.status)
}

// HaveAllAdaptersAtGeneration matches an *AdapterStatusList where every required
// adapter has observed the given generation with Applied=True, Available=True, Health=True.
// requiredAdapters is the name→URL map from AdaptersConfig; only the keys (names) are used.
func HaveAllAdaptersAtGeneration(requiredAdapters map[string]string, generation int32) types.GomegaMatcher {
	return &allAdaptersGenerationMatcher{
		adapters:   requiredAdapters,
		generation: generation,
	}
}

type allAdaptersGenerationMatcher struct {
	adapters   map[string]string
	generation int32
	failures   []string
}

func (m *allAdaptersGenerationMatcher) Match(actual any) (bool, error) {
	list, ok := actual.(*client.AdapterStatusList)
	if !ok {
		return false, fmt.Errorf("HaveAllAdaptersAtGeneration expects *AdapterStatusList, got %T", actual)
	}
	if list == nil {
		return false, fmt.Errorf("HaveAllAdaptersAtGeneration expects non-nil *AdapterStatusList")
	}

	m.failures = nil
	adapterMap := make(map[string]client.AdapterStatus, len(list.Items))
	for _, s := range list.Items {
		adapterMap[s.Adapter] = s
	}

	for name := range m.adapters {
		adapter, exists := adapterMap[name]
		if !exists {
			m.failures = append(m.failures, name+": not found")
			continue
		}
		if adapter.ObservedGeneration != m.generation {
			m.failures = append(m.failures, fmt.Sprintf("%s: generation %d (want %d)", name, adapter.ObservedGeneration, m.generation))
			continue
		}
		for _, ct := range []string{client.ConditionTypeApplied, client.ConditionTypeAvailable, client.ConditionTypeHealth} {
			if !hasAdapterCond(adapter.Conditions, ct, client.AdapterConditionStatusTrue) {
				m.failures = append(m.failures, fmt.Sprintf("%s: %s!=True", name, ct))
			}
		}
	}
	return len(m.failures) == 0, nil
}

func (m *allAdaptersGenerationMatcher) FailureMessage(_ any) string {
	return fmt.Sprintf("adapters not at generation %d: %s", m.generation, strings.Join(m.failures, "; "))
}

func (m *allAdaptersGenerationMatcher) NegatedFailureMessage(_ any) string {
	return fmt.Sprintf("expected adapters NOT at generation %d", m.generation)
}

// HaveAuditIdentity matches a *Resource whose CreatedBy field equals the expected identity.
func HaveAuditIdentity(expected string) types.GomegaMatcher {
	return &auditIdentityMatcher{expected: expected}
}

type auditIdentityMatcher struct {
	expected string
	actual   string
}

func (m *auditIdentityMatcher) Match(actual any) (bool, error) {
	identity, err := extractCreatedBy(actual)
	if err != nil {
		return false, err
	}
	m.actual = identity
	return identity == m.expected, nil
}

func (m *auditIdentityMatcher) FailureMessage(_ any) string {
	return fmt.Sprintf("expected created_by=%q but got %q", m.expected, m.actual)
}

func (m *auditIdentityMatcher) NegatedFailureMessage(_ any) string {
	return fmt.Sprintf("expected created_by NOT to be %q", m.expected)
}

func extractCreatedBy(actual any) (string, error) {
	v, ok := actual.(*client.Resource)
	if !ok {
		return "", fmt.Errorf("HaveAuditIdentity expects *Resource, got %T", actual)
	}
	if v == nil {
		return "", fmt.Errorf("HaveAuditIdentity expects non-nil *Resource")
	}
	if v.CreatedBy == nil {
		return "", nil
	}
	return *v.CreatedBy, nil
}

func hasAdapterCond(conditions []client.AdapterCondition, condType string, status client.AdapterConditionStatus) bool {
	for _, c := range conditions {
		if c.Type == condType && c.Status == status {
			return true
		}
	}
	return false
}

func extractResourceConditions(actual any) ([]client.ResourceCondition, error) {
	v, ok := actual.(*client.Resource)
	if !ok {
		return nil, fmt.Errorf("HaveResourceCondition expects *Resource, got %T", actual)
	}
	if v == nil {
		return nil, nil
	}
	return v.Status.Conditions, nil
}

func formatResourceConditions(conditions []client.ResourceCondition) string {
	if len(conditions) == 0 {
		return "<no conditions>"
	}
	parts := make([]string, 0, len(conditions))
	for _, c := range conditions {
		parts = append(parts, fmt.Sprintf("%s=%s", c.Type, c.Status))
	}
	return strings.Join(parts, ", ")
}
