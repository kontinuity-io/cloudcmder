package tui

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoProviderImports enforces the architecture dependency rule: no file
// under internal/tui/... may import internal/providers/*. Implemented as a
// Go test instead of a golangci-lint depguard rule so it runs locally on
// `go test ./...` and survives any future linter config churn.
func TestNoProviderImports(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	walkErr := filepath.WalkDir(wd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			v := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(v, "cloudcmder.com/internal/providers") {
				t.Errorf("%s imports forbidden package %q — internal/tui must read only from store",
					path, v)
			}
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
}
