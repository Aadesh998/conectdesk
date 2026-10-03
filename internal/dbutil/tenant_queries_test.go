package dbutil

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/knadh/goyesql/v2"
)

// tenantAllMarker exempts a query from tenant filtering. It must say why, e.g.
// `-- tenant: all, the worker claims jobs of every tenant and reads tenant_id`.
const tenantAllMarker = "-- tenant: all"

var tenantIDRe = regexp.MustCompile(`\btenant_id\b`)

// TestQueriesAreTenantScoped fails for any named query that doesn't mention
// tenant_id, so a query can't silently read or write across tenants. Queries
// that deliberately span tenants carry the tenantAllMarker comment.
func TestQueriesAreTenantScoped(t *testing.T) {
	var files []string
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".sql") {
			files = append(files, path)
		}
		return err
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("finding query files: %v", err)
	}

	var missing []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		queries, err := goyesql.ParseBytes(b)
		if err != nil {
			t.Fatalf("parsing %s: %v", f, err)
		}
		raw := string(b)
		for name, q := range queries {
			if tenantIDRe.MatchString(q.Query) || exempt(raw, name) {
				continue
			}
			rel, _ := filepath.Rel("..", filepath.Dir(f))
			missing = append(missing, rel+"/"+name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d queries don't filter by tenant_id:\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
}

// exempt reports whether the named query's block in raw carries the marker.
func exempt(raw, name string) bool {
	i := strings.Index(raw, "-- name: "+name+"\n")
	if i < 0 {
		return false
	}
	block := raw[i+len("-- name: "+name+"\n"):]
	if j := strings.Index(block, "-- name: "); j >= 0 {
		block = block[:j]
	}
	return strings.Contains(block, tenantAllMarker)
}
