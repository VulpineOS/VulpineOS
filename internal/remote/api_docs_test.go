package remote

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func TestReadmePanelControlMessageCountMatchesDispatcher(t *testing.T) {
	root := repoRoot(t)
	count := countPanelControlCases(t, filepath.Join(root, "internal", "remote", "api.go"))

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	re := regexp.MustCompile(`(\d+) control messages`)
	matches := re.FindAllSubmatch(readme, -1)
	if len(matches) == 0 {
		t.Fatal("README has no panel control-message count")
	}
	for _, match := range matches {
		got, err := strconv.Atoi(string(match[1]))
		if err != nil {
			t.Fatalf("parse README count %q: %v", match[1], err)
		}
		if got != count {
			t.Fatalf("README control-message count = %d, dispatcher has %d", got, count)
		}
	}
}

func countPanelControlCases(t *testing.T, path string) int {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, 0)
	if err != nil {
		t.Fatalf("parse api.go: %v", err)
	}
	var count int
	ast.Inspect(file, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "HandleMessage" {
			return true
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			clause, ok := node.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				if lit, ok := expr.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					count++
				}
			}
			return true
		})
		return false
	})
	if count == 0 {
		t.Fatal("no HandleMessage control cases found")
	}
	return count
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
