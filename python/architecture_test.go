package python

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestManagedCommandsHaveOneExecutorBoundary(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") || path == "runtime.go" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, data, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range directExecutorCalls(parsed) {
			if allowedUnmanagedExecutorCall(path, call) {
				continue
			}
			t.Errorf("%s contains executor.%s outside the managed child-command boundary", path, executorCallName(call))
		}
	}
}

func directExecutorCalls(file *ast.File) []*ast.CallExpr {
	var calls []*ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || (selector.Sel.Name != "Run" && selector.Sel.Name != "RunVerbose") {
			return true
		}
		packageName, ok := selector.X.(*ast.Ident)
		if ok && packageName.Name == "executor" {
			calls = append(calls, call)
		}
		return true
	})
	return calls
}

func allowedUnmanagedExecutorCall(path string, call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	literal, ok := call.Args[1].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return false
	}
	command, err := strconv.Unquote(literal.Value)
	if err != nil {
		return false
	}
	allowed := map[string][]string{
		"cleanup.go": {"apt-cache", "dpkg-query"},
		"venv.go":    {"git"},
	}
	return slices.Contains(allowed[filepath.Base(path)], command)
}

func executorCallName(call *ast.CallExpr) string {
	selector, _ := call.Fun.(*ast.SelectorExpr)
	return selector.Sel.Name
}
