package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryGoroutineRecovers scans all non-test Go code of the repository:
// every go statement and every time.AfterFunc callback must be a function
// literal that defers a recovery (recoverAndLog, crashlog.Recover or a
// deferred func calling recover()) before doing any work, so a panic in a
// goroutine is written to the crash log instead of closing the app. The
// recovery helpers must be deferred directly: called from inside another
// function, recover() sees no panic.
func TestEveryGoroutineRecovers(t *testing.T) {
	fset := token.NewFileSet()
	files, goStmts, timers := 0, 0, 0
	err := filepath.WalkDir(repoRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "build", "dist", ".mygo", ".git", "node_modules", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, 0)
		if err != nil {
			return err
		}
		files++
		// Calls that are directly deferred.
		deferred := map[*ast.CallExpr]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			if ds, ok := n.(*ast.DeferStmt); ok {
				deferred[ds.Call] = true
			}
			return true
		})
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.GoStmt:
				goStmts++
				if !protectedFunc(n.Call.Fun) {
					t.Errorf("%s: go statement without a recovery", fset.Position(n.Pos()))
				}
			case *ast.CallExpr:
				if isSel(n.Fun, "time", "AfterFunc") && len(n.Args) == 2 {
					timers++
					if !protectedFunc(n.Args[1]) {
						t.Errorf("%s: time.AfterFunc callback without a recovery", fset.Position(n.Pos()))
					}
				}
				if (isSel(n.Fun, "crashlog", "Recover") || isIdent(n.Fun, "recoverAndLog")) && !deferred[n] {
					t.Errorf("%s: recovery helper not deferred directly (recover() would see no panic)", fset.Position(n.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files < 60 || goStmts < 12 || timers < 2 {
		t.Fatalf("scanned only %d files, %d go statements, %d timers", files, goStmts, timers)
	}
}

// protectedFunc reports whether fn is a function literal that defers a
// recovery before any statement that calls something.
func protectedFunc(fn ast.Expr) bool {
	lit, ok := fn.(*ast.FuncLit)
	if !ok {
		return false
	}
	for _, st := range lit.Body.List {
		if ds, ok := st.(*ast.DeferStmt); ok {
			if recovering(ds.Call) {
				return true
			}
			continue
		}
		if hasCall(st) {
			return false
		}
	}
	return false
}

func recovering(c *ast.CallExpr) bool {
	if isIdent(c.Fun, "recoverAndLog") || isSel(c.Fun, "crashlog", "Recover") {
		return true
	}
	lit, ok := c.Fun.(*ast.FuncLit)
	if !ok {
		return false
	}
	found := false
	ast.Inspect(lit.Body, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && isIdent(call.Fun, "recover") {
			found = true
		}
		return !found
	})
	return found
}

func hasCall(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		if _, ok := n.(*ast.CallExpr); ok {
			found = true
		}
		return !found
	})
	return found
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isSel(e ast.Expr, pkg, name string) bool {
	s, ok := e.(*ast.SelectorExpr)
	return ok && s.Sel.Name == name && isIdent(s.X, pkg)
}
