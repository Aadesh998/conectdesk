package export

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/conectdesk/conectdesk/internal/migrations"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/zerodha/logf"
)

type fakeFiles struct {
	blobs map[string][]byte
	saved map[string][]byte
}

func (f *fakeFiles) Blob(name string) ([]byte, error) {
	b, ok := f.blobs[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return b, nil
}

func (f *fakeFiles) Save(tenantID, exportID int, name string, file *os.File, size int64) (string, error) {
	b, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	f.saved["11111111-1111-1111-1111-111111111111"] = b
	return "11111111-1111-1111-1111-111111111111", nil
}

func (f *fakeFiles) Remove(tenantID int, uuid string) error { delete(f.saved, uuid); return nil }

func TestExport(t *testing.T) {
	db := testutil.NewDB(t, "export")
	db.MustExec(`INSERT INTO tenants (id, slug, "name") VALUES (2, 'acme', 'Acme'), (3, 'globex', 'Globex')`)
	db.MustExec(`INSERT INTO users (tenant_id, type, first_name, last_name, email, password, api_key, api_secret, totp_secret, totp_recovery_codes)
		VALUES (2, 'agent', 'Alice', '', 'alice@acme.io', 'bcrypt-hash', 'key-1', 'secret-hash', 'enc-totp', '{"h1"}'),
		       (3, 'agent', 'Gina', '', 'gina@globex.io', 'x', NULL, NULL, NULL, '{}')`)
	db.MustExec(`INSERT INTO settings (tenant_id, key, value) VALUES
		(2, 'notification.email.password', '"smtp-pass"'), (2, 'app.site_name', '"Acme Desk"')`)
	db.MustExec(`INSERT INTO inboxes (tenant_id, name, channel, config, "from")
		VALUES (2, 'Support', 'email', '{"imap":[{"host":"imap.acme.io","password":"imap-pass"}],"oauth":{"refresh_token":"rt","client_id":"cid"}}', 'support@acme.io')`)
	db.MustExec(`INSERT INTO media (tenant_id, "uuid", store, filename, content_type, "size", model_type) VALUES
		(2, 'aaaaaaaa-0000-0000-0000-000000000001', 'fs', 'invoice.pdf', 'application/pdf', 5, 'messages'),
		(2, 'aaaaaaaa-0000-0000-0000-000000000002', 'fs', '../big.bin', 'application/octet-stream', 50, 'messages'),
		(2, 'aaaaaaaa-0000-0000-0000-000000000003', 'fs', 'old-export.zip', 'application/zip', 3, 'data_exports')`)

	lo := logf.New(logf.Opts{})
	files := &fakeFiles{
		blobs: map[string][]byte{
			"aaaaaaaa-0000-0000-0000-000000000001": []byte("%PDF!"),
			"aaaaaaaa-0000-0000-0000-000000000002": make([]byte, 50),
			"aaaaaaaa-0000-0000-0000-000000000003": []byte("zip"),
		},
		saved: map[string][]byte{},
	}
	tables := slices.Concat(migrations.TenantScopedTables, migrations.TenantScopedTablesV2_14_0, migrations.TenantScopedTablesV2_16_0, migrations.TenantScopedTablesV2_20_0)
	m, err := New(Opts{DB: db, Lo: &lo, Files: files, Tables: tables, MaxFilesBytes: 20})
	if err != nil {
		t.Fatal(err)
	}

	e, err := m.Start(context.Background(), 2, 1, "alice@acme.io", "acme.zip")
	if err != nil {
		t.Fatal(err)
	}
	m.Wait()
	got, _ := m.Get(2, e.UUID)
	if got.Status != "ready" || got.Size == 0 {
		t.Fatalf("export: %+v", got)
	}
	if _, err := m.Get(3, e.UUID); err == nil {
		t.Fatal("globex sees acme's export")
	}
	if u, err := m.DownloadMedia(2, e.UUID); err != nil || u == "" {
		t.Fatalf("download: %q %v", u, err)
	}

	b := files.saved["11111111-1111-1111-1111-111111111111"]
	zr, err := zip.NewReader(strings.NewReader(string(b)), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		c, _ := io.ReadAll(rc)
		all[f.Name] = string(c)
	}
	for _, want := range []string{"users.jsonl", "settings.jsonl", "inboxes.jsonl", "media.jsonl", "manifest.json", "files/aaaaaaaa-0000-0000-0000-000000000001/invoice.pdf"} {
		if _, ok := all[want]; !ok {
			t.Errorf("missing %s", want)
		}
	}
	everything := strings.Join(slices.Collect(func(yield func(string) bool) {
		for _, v := range all {
			if !yield(v) {
				return
			}
		}
	}), "\n")
	for _, secret := range []string{"bcrypt-hash", "secret-hash", "enc-totp", "smtp-pass", "imap-pass", `"rt"`, "gina@globex.io"} {
		if strings.Contains(everything, secret) {
			t.Errorf("export contains %q", secret)
		}
	}
	for _, kept := range []string{"alice@acme.io", "Acme Desk", "imap.acme.io", "cid", "%PDF!"} {
		if !strings.Contains(everything, kept) {
			t.Errorf("export lacks %q", kept)
		}
	}
	if _, ok := all["files/aaaaaaaa-0000-0000-0000-000000000003/old-export.zip"]; ok {
		t.Error("an earlier export is inside the export")
	}
	if !strings.Contains(all["manifest.json"], `"files_skipped": 1`) {
		t.Errorf("manifest: %s", all["manifest.json"])
	}

	// One export at a time.
	db.MustExec(`UPDATE data_exports SET status = 'running' WHERE tenant_id = 2`)
	if _, err := m.Start(context.Background(), 2, 1, "", "x.zip"); !errors.Is(err, ErrRunning) {
		t.Fatalf("second export: %v", err)
	}
	if _, err := m.Start(context.Background(), 3, 1, "", "g.zip"); err != nil {
		t.Fatalf("globex blocked by acme's export: %v", err)
	}
	m.Wait()

	// Expired exports lose their file.
	db.MustExec(`UPDATE data_exports SET status = 'ready', expires_at = NOW() - INTERVAL '1 minute' WHERE tenant_id = 2`)
	m.RemoveExpired()
	if got, _ := m.Get(2, e.UUID); got.Status != "expired" {
		t.Fatalf("after expiry: %s", got.Status)
	}
}

func TestRedact(t *testing.T) {
	out, _ := Redact([]byte(`{"id":1,"token_hash":"keep?","smtp":{"password":"p","host":"h"},"list":[{"api_key":"k"}],"password":"","config":"{\"secret\":\"s\"}"}`))
	s := string(out)
	for _, bad := range []string{`"p"`, `"k"`, `\"s\"`} {
		if strings.Contains(s, bad) {
			t.Errorf("not redacted %s in %s", bad, s)
		}
	}
	if !strings.Contains(s, `"host":"h"`) || !strings.Contains(s, `"password":""`) {
		t.Errorf("redacted too much: %s", s)
	}
}
