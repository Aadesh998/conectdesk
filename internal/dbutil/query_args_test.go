package dbutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/knadh/goyesql/v2"
)

var placeholderRe = regexp.MustCompile(`\$(\d+)`)

// stmtMethods are the sqlx.Stmt methods whose arguments are query parameters,
// with how many leading arguments aren't (a context, a destination).
var stmtMethods = map[string]int{
	"Exec": 0, "Query": 0, "Queryx": 0, "QueryRow": 0, "QueryRowx": 0,
	"Get": 1, "Select": 1,
	"ExecContext": 1, "QueryContext": 1, "QueryxContext": 1, "QueryRowContext": 1, "QueryRowxContext": 1,
	"GetContext": 2, "SelectContext": 2,
}

// TestPreparedStatementArgs checks every call of a prepared statement passes as
// many parameters as its query has placeholders. The compiler can't see this,
// and a mismatch only fails at runtime.
func TestPreparedStatementArgs(t *testing.T) {
	var dirs []string
	filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && filepath.Base(path) == "queries.sql" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return err
	})

	checked := 0
	for _, dir := range dirs {
		b, err := os.ReadFile(filepath.Join(dir, "queries.sql"))
		if err != nil {
			t.Fatal(err)
		}
		queries, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatal(err)
		}

		fset := token.NewFileSet()
		pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pkg := range pkgs {
			fields := stmtFields(pkg)
			for _, f := range pkg.Files {
				ast.Inspect(f, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok || call.Ellipsis.IsValid() {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					// A statement handed to a helper, as in h(x.q.Field, a, b), runs
					// with the helper's other arguments.
					if sel.Sel.Name != "Stmtx" {
						for _, arg := range call.Args {
							name, ok := fields[stmtField(arg)]
							if q, found := queries[name]; ok && found {
								checked++
								if got, want := len(call.Args)-1, maxPlaceholder(q.Query); got != want {
									t.Errorf("%s: %s (%s) goes to %s with %d parameters, the query takes %d",
										fset.Position(call.Pos()), stmtField(arg), name, sel.Sel.Name, got, want)
								}
							}
						}
					}
					skip, ok := stmtMethods[sel.Sel.Name]
					if !ok {
						return true
					}
					field := stmtField(sel.X)
					name, ok := fields[field]
					if !ok {
						return true
					}
					q, ok := queries[name]
					if !ok {
						return true
					}
					want := maxPlaceholder(q.Query)
					got := len(call.Args) - skip
					checked++
					if got != want {
						t.Errorf("%s: %s (%s) passes %d parameters, the query takes %d",
							fset.Position(call.Pos()), field, name, got, want)
					}
					return true
				})
			}
		}
	}
	if checked == 0 {
		t.Fatal("found no prepared statement calls")
	}
}

// stmtFields maps each *sqlx.Stmt struct field tagged `query:"name"` to the query name.
func stmtFields(pkg *ast.Package) map[string]string {
	out := map[string]string{}
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fl := range st.Fields.List {
				star, ok := fl.Type.(*ast.StarExpr)
				if !ok || fl.Tag == nil || len(fl.Names) != 1 {
					continue
				}
				if sel, ok := star.X.(*ast.SelectorExpr); !ok || sel.Sel.Name != "Stmt" {
					continue
				}
				tag, _ := strconv.Unquote(fl.Tag.Value)
				if name := reflect.StructTag(tag).Get("query"); name != "" {
					out[fl.Names[0].Name] = name
				}
			}
			return true
		})
	}
	return out
}

// stmtField returns the statement field a call runs on: `x.q.Field` or
// `tx.Stmtx(x.q.Field)`.
func stmtField(e ast.Expr) string {
	if call, ok := e.(*ast.CallExpr); ok {
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Stmtx" && len(call.Args) == 1 {
			e = call.Args[0]
		}
	}
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	if inner, ok := sel.X.(*ast.SelectorExpr); ok && (inner.Sel.Name == "q" || inner.Sel.Name == "queries") {
		return sel.Sel.Name
	}
	return ""
}

func maxPlaceholder(q string) int {
	max := 0
	for _, m := range placeholderRe.FindAllStringSubmatch(q, -1) {
		if n, _ := strconv.Atoi(m[1]); n > max {
			max = n
		}
	}
	return max
}
