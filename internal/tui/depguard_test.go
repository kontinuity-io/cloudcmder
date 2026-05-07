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
// under internal/tui/... or internal/export/... may import
// internal/providers/*. Implemented as a Go test instead of a golangci-lint
// depguard rule so it runs locally on `go test ./...` and survives any
// future linter config churn.
func TestNoProviderImports(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd is internal/tui at test runtime. Walk it AND the sibling
	// internal/export/ package so both store-only consumers stay clean.
	roots := []string{wd, filepath.Join(wd, "..", "export")}

	for _, root := range roots {
		walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return filepath.SkipDir
				}
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
					t.Errorf("%s imports forbidden package %q — store-only consumers must not depend on providers",
						path, v)
				}
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", root, walkErr)
		}
	}
}
