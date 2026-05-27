package tui

import (
	"regexp"
	"strings"
)

// matchSuitesByCustomFocus finds discovered suites for a custom focus string.
// It tries regex match on Describe names first, then case-insensitive word match.
// Matched suites keep Focus empty so ginkgoFocus uses the full Describe name.
func matchSuitesByCustomFocus(focus string, suites []TestSuite) []TestSuite {
	focus = strings.TrimSpace(focus)
	if focus == "" {
		return nil
	}

	if re, err := regexp.Compile(focus); err == nil {
		var matched []TestSuite
		for _, s := range suites {
			if re.MatchString(s.Name) {
				matched = append(matched, cloneDiscoveredSuite(s))
			}
		}
		if len(matched) > 0 {
			return matched
		}
	}

	words := focusWords(focus)
	if len(words) == 0 {
		return nil
	}

	var matched []TestSuite
	for _, s := range suites {
		if allWordsInString(words, s.Name) {
			matched = append(matched, cloneDiscoveredSuite(s))
		}
	}
	return matched
}

// ginkgoFocusFromCustomFocus builds a ginkgo --focus regex when no Describe matched.
func ginkgoFocusFromCustomFocus(focus string) string {
	words := focusWords(focus)
	if len(words) == 0 {
		return focus
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = regexp.QuoteMeta(w)
	}
	return "(?i)" + strings.Join(parts, ".*")
}

func cloneDiscoveredSuite(s TestSuite) TestSuite {
	return TestSuite{
		Name:   s.Name,
		File:   s.File,
		Labels: append([]string(nil), s.Labels...),
	}
}

func focusWords(focus string) []string {
	return strings.Fields(strings.TrimSpace(focus))
}

func allWordsInString(words []string, s string) bool {
	lower := strings.ToLower(s)
	for _, w := range words {
		if !strings.Contains(lower, strings.ToLower(w)) {
			return false
		}
	}
	return true
}
