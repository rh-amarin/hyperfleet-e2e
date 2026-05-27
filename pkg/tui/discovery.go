package tui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// TestSuite represents a runnable test unit for the TUI.
type TestSuite struct {
	Name   string   // display name
	File   string   // source file (discovery only)
	Labels []string // ginkgo labels on this container
	Focus  string   // ginkgo --focus regex; empty means match Name exactly
}

// labelConstToValue maps labels package constant names to their string values.
var labelConstToValue = map[string]string{
	"Tier0":       "tier0",
	"Tier1":       "tier1",
	"Tier2":       "tier2",
	"Negative":    "negative",
	"Performance": "perf",
	"Upgrade":     "upgrade",
	"Disruptive":  "disruptive",
	"Slow":        "slow",
}

// DiscoverSuites walks the e2e directory and extracts all top-level ginkgo.Describe blocks.
func DiscoverSuites(e2eDir string) ([]TestSuite, error) {
	fset := token.NewFileSet()
	var suites []TestSuite

	err := filepath.WalkDir(e2eDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil // skip unparseable files
		}

		rel, _ := filepath.Rel(e2eDir, path)

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Match: var _ = ginkgo.Describe(...)
			// The outer AssignStmt is handled by walking — we just need CallExpr.
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkg.Name != "ginkgo" || sel.Sel.Name != "Describe" {
				return true
			}

			if len(call.Args) < 2 {
				return true
			}

			// First arg must be a string literal
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			name := strings.Trim(lit.Value, `"`)

			// Collect labels from any ginkgo.Label(...) arg
			labels := extractLabels(call.Args[1:])

			suites = append(suites, TestSuite{
				Name:   name,
				File:   rel,
				Labels: labels,
			})
			return true
		})
		return nil
	})

	return suites, err
}

// extractLabels finds ginkgo.Label(...) calls in args and resolves label constants.
func extractLabels(args []ast.Expr) []string {
	var labels []string
	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}
		if pkg.Name != "ginkgo" || sel.Sel.Name != "Label" {
			continue
		}
		for _, labelArg := range call.Args {
			label := resolveLabel(labelArg)
			if label != "" {
				labels = append(labels, label)
			}
		}
	}
	return labels
}

// resolveLabel converts a label AST node to its string value.
// Handles string literals and labels.Const selector expressions.
func resolveLabel(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return strings.Trim(v.Value, `"`)
		}
	case *ast.SelectorExpr:
		if id, ok := v.X.(*ast.Ident); ok && id.Name == "labels" {
			if val, known := labelConstToValue[v.Sel.Name]; known {
				return val
			}
		}
	}
	return ""
}

// FilterByLabel returns suites that have the given label. Empty label = all suites.
func FilterByLabel(suites []TestSuite, label string) []TestSuite {
	if label == "" {
		return suites
	}
	var out []TestSuite
	for _, s := range suites {
		for _, l := range s.Labels {
			if l == label {
				out = append(out, s)
				break
			}
		}
	}
	return out
}

// HasLabel returns true if the suite has the given label.
func (s TestSuite) HasLabel(label string) bool {
	for _, l := range s.Labels {
		if l == label {
			return true
		}
	}
	return false
}
