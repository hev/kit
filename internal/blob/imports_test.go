package blob

import (
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Trace capture reuses this package, so it must not depend on the board.
func TestNoBoardImports(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		ast, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, im := range ast.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if strings.Contains(p, "/internal/") && p != "github.com/hev/kit/internal/platformops" {
				t.Errorf("%s imports %s; blob may import only platformops from kit", f, p)
			}
		}
	}
}
