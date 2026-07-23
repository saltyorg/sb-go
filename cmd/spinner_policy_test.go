package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

func TestCommandRootsRetainChildrenUnlessExplicitlyAllowed(t *testing.T) {
	t.Helper()

	allowedCollapse := map[string]bool{
		"dockerRestart.go": true,
		"dockerStart.go":   true,
		"dockerStop.go":    true,
	}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fileset := token.NewFileSet()
	for _, filename := range files {
		if filepath.Ext(filename) != ".go" || filepath.Base(filename) == "spinner_policy_test.go" {
			continue
		}
		file, err := parser.ParseFile(fileset, filename, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", filename, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 || !isRunnerRunCall(call.Fun) {
				return true
			}
			spec, ok := call.Args[1].(*ast.CompositeLit)
			if !ok || !taskSpecCollapsesChildren(spec) {
				return true
			}
			if !allowedCollapse[filepath.Base(filename)] {
				t.Errorf("%s:%d: command root collapses children without an explicit policy exception",
					filename, fileset.Position(call.Pos()).Line)
			}
			return true
		})
	}
}

func isRunnerRunCall(expression ast.Expr) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Run" {
		return false
	}
	receiver, ok := selector.X.(*ast.Ident)
	return ok && receiver.Name == "runner"
}

func taskSpecCollapsesChildren(spec *ast.CompositeLit) bool {
	for _, element := range spec.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		name, ok := field.Key.(*ast.Ident)
		if !ok || name.Name != "ChildDisplay" {
			continue
		}
		selector, ok := field.Value.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == "CollapseChildTasks"
	}
	return false
}
