package dispatch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Keep new tmux call sites from silently bypassing the suite's private socket.
// Direct calls in integration tests must themselves specify a private -S socket.
func TestNoUnisolatedTmuxCommands(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		var defaultConstructor ast.Node
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if ok && path == "process_unix.go" && len(value.Names) == 1 && value.Names[0].Name == "tmuxCommand" {
					defaultConstructor = value
				}
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if node == defaultConstructor {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok || pkg.Name != "exec" {
				return true
			}
			offset := 0
			switch sel.Sel.Name {
			case "Command":
			case "CommandContext":
				offset = 1
			default:
				return true
			}
			literal := func(i int) string {
				if i >= len(call.Args) {
					return ""
				}
				s, ok := call.Args[i].(*ast.BasicLit)
				if !ok || s.Kind != token.STRING {
					return ""
				}
				value, _ := strconv.Unquote(s.Value)
				return value
			}
			if literal(offset) == "tmux" && (!strings.HasSuffix(path, "_test.go") || literal(offset+1) != "-S") {
				t.Errorf("%s: tmux bypasses private socket; use tmuxCommand", fset.Position(call.Pos()))
			}
			return true
		})
	}
}
