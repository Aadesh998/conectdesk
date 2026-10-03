// Package export builds a ZIP of all of a tenant's data, for the tenant to
// keep or move elsewhere: every tenant table as JSON Lines with credentials
// redacted, the files it stores, and a manifest.
package export

import (
	"archive/zip"
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/logf"
)

var (
	//go:embed queries.sql
	efs embed.FS

	// ErrRunning is returned when the tenant already has an export running.
	ErrRunning = errors.New("an export is already running")
	// ErrNotReady is returned for an export that isn't ready to download.
	ErrNotReady = errors.New("export not ready")

	// sensitiveKey matches the names of columns and JSON keys holding
	// credentials, whose values are redacted.
	sensitiveKey = regexp.MustCompile(`(?i)(^|[._])(password|passwd|secret|client_secret|api_secret|api_key|access_token|refresh_token|token|totp_secret|totp_recovery_codes|reset_password_token|private_key|encryption_key)$`)
)

const (
	// ModelType is the media model_type of export files.
	ModelType = "data_exports"
	redacted  = "[redacted]"
)

// skipTables aren't exported: search vectors are derived from the rest, and
// exports are about the data, not about exporting it.
var skipTables = []string{"embeddings", "data_exports"}

// Export is one export of a tenant's data.
type Export struct {
	ID               int             `db:"id" json:"-"`
	UUID             string          `db:"uuid" json:"uuid"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	FinishedAt       null.Time       `db:"finished_at" json:"finished_at"`
	ExpiresAt        null.Time       `db:"expires_at" json:"expires_at"`
	Status           string          `db:"status" json:"status"`
	RequestedByEmail string          `db:"requested_by_email" json:"requested_by_email"`
	MediaUUID        null.String     `db:"media_uuid" json:"-"`
	Size             int64           `db:"size" json:"size"`
	Manifest         json.RawMessage `db:"manifest" json:"manifest"`
	Error            string          `db:"error" json:"error"`
}

// Manifest describes an export's contents.
type Manifest struct {
	TenantID     int            `json:"workspace_id"`
	CreatedAt    time.Time      `json:"created_at"`
	Tables       map[string]int `json:"tables"`
	Files        int            `json:"files"`
	FilesSkipped int            `json:"files_skipped"`
	FilesBytes   int64          `json:"files_bytes"`
	Note         string         `json:"note,omitempty"`
}

// Files reads and stores files.
type Files interface {
	// Blob returns a stored file's content by its name.
	Blob(name string) ([]byte, error)
	// Save stores the export ZIP for the tenant and returns its media uuid.
	Save(tenantID, exportID int, name string, f *os.File, size int64) (string, error)
	// Remove deletes a stored export ZIP.
	Remove(tenantID int, mediaUUID string) error
}

// Opts contains options for initializing the Manager.
type Opts struct {
	DB    *sqlx.DB
	Lo    *logf.Logger
	Files Files
	// Tables lists the tenant tables to export.
	Tables []string
	// MaxFilesBytes caps the stored files an export includes; 0 includes none.
	MaxFilesBytes int64
	// TTL is how long a finished export can be downloaded.
	TTL time.Duration
}

type queries struct {
	InsertExport      *sqlx.Stmt `query:"insert-export"`
	GetExports        *sqlx.Stmt `query:"get-exports"`
	GetExport         *sqlx.Stmt `query:"get-export"`
	SetExportReady    *sqlx.Stmt `query:"set-export-ready"`
	SetExportFailed   *sqlx.Stmt `query:"set-export-failed"`
	GetExpiredExports *sqlx.Stmt `query:"get-expired-exports"`
	SetExportExpired  *sqlx.Stmt `query:"set-export-expired"`
}

// Manager runs exports.
type Manager struct {
	q     queries
	db    *sqlx.DB
	lo    *logf.Logger
	files Files
	opts  Opts
	wg    sync.WaitGroup
}

// New returns a Manager.
func New(opts Opts) (*Manager, error) {
	var q queries
	if err := dbutil.ScanSQLFile("queries.sql", &q, opts.DB, efs); err != nil {
		return nil, err
	}
	if opts.TTL <= 0 {
		opts.TTL = 7 * 24 * time.Hour
	}
	return &Manager{q: q, db: opts.DB, lo: opts.Lo, files: opts.Files, opts: opts}, nil
}

// Start begins exporting the tenant's data in the background and returns the
// export, running.
func (m *Manager) Start(ctx context.Context, tenantID, userID int, email, fileName string) (Export, error) {
	var e Export
	if err := m.q.InsertExport.Get(&e, tenantID, userID, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return e, ErrRunning
		}
		m.lo.Error("error starting export", "tenant_id", tenantID, "error", err)
		return e, err
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		if err := m.run(ctx, tenantID, e.ID, fileName); err != nil {
			m.lo.Error("export failed", "tenant_id", tenantID, "export_id", e.ID, "error", err)
			m.q.SetExportFailed.Exec(tenantID, e.ID, err.Error())
		}
	}()
	return e, nil
}

// Wait waits for running exports to finish.
func (m *Manager) Wait() { m.wg.Wait() }

// List returns the tenant's latest exports.
func (m *Manager) List(tenantID int) ([]Export, error) {
	out := []Export{}
	if err := m.q.GetExports.Select(&out, tenantID); err != nil {
		return nil, err
	}
	return out, nil
}

// Get returns one of the tenant's exports.
func (m *Manager) Get(tenantID int, uuid string) (Export, error) {
	var e Export
	err := m.q.GetExport.Get(&e, tenantID, uuid)
	return e, err
}

// DownloadMedia returns the media uuid of a ready export's ZIP.
func (m *Manager) DownloadMedia(tenantID int, uuid string) (string, error) {
	e, err := m.Get(tenantID, uuid)
	if err != nil || e.Status != "ready" || !e.MediaUUID.Valid {
		return "", ErrNotReady
	}
	return e.MediaUUID.String, nil
}

// RemoveExpired deletes the files of expired exports.
func (m *Manager) RemoveExpired() {
	var rows []struct {
		TenantID  int         `db:"tenant_id"`
		ID        int         `db:"id"`
		MediaUUID null.String `db:"media_uuid"`
	}
	if err := m.q.GetExpiredExports.Select(&rows); err != nil {
		m.lo.Error("error fetching expired exports", "error", err)
		return
	}
	for _, r := range rows {
		if r.MediaUUID.Valid {
			if err := m.files.Remove(r.TenantID, r.MediaUUID.String); err != nil {
				m.lo.Error("error removing expired export", "tenant_id", r.TenantID, "export_id", r.ID, "error", err)
				continue
			}
		}
		m.q.SetExportExpired.Exec(r.TenantID, r.ID)
	}
}

// RunCleaner removes expired exports every hour until ctx is done.
func (m *Manager) RunCleaner(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		m.RemoveExpired()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (m *Manager) run(ctx context.Context, tenantID, exportID int, fileName string) error {
	tmp, err := os.CreateTemp("", "conectdesk-export-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()

	man := Manifest{TenantID: tenantID, CreatedAt: time.Now().UTC(), Tables: map[string]int{}}
	zw := zip.NewWriter(tmp)
	for _, table := range m.opts.Tables {
		if slices.Contains(skipTables, table) {
			continue
		}
		n, err := m.writeTable(ctx, zw, tenantID, table)
		if err != nil {
			return fmt.Errorf("exporting %s: %w", table, err)
		}
		man.Tables[table] = n
	}
	if err := m.writeFiles(ctx, zw, tenantID, &man); err != nil {
		return fmt.Errorf("exporting files: %w", err)
	}
	if man.FilesSkipped > 0 {
		man.Note = "Some files were left out to keep the export within its size limit; media.jsonl lists every file."
	}
	b, _ := json.MarshalIndent(man, "", "  ")
	w, err := zw.Create("manifest.json")
	if err != nil {
		return err
	}
	w.Write(b)
	if err := zw.Close(); err != nil {
		return err
	}

	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return err
	}
	mediaUUID, err := m.files.Save(tenantID, exportID, fileName, tmp, size)
	if err != nil {
		return fmt.Errorf("storing export: %w", err)
	}
	b, _ = json.Marshal(man)
	_, err = m.q.SetExportReady.Exec(tenantID, exportID, mediaUUID, size, m.opts.TTL.Hours(), b)
	return err
}

// writeTable writes the tenant's rows of table as <table>.jsonl and returns
// how many there were.
func (m *Manager) writeTable(ctx context.Context, zw *zip.Writer, tenantID int, table string) (int, error) {
	rows, err := m.db.QueryxContext(ctx, `SELECT row_to_json(t)::text FROM `+pq.QuoteIdentifier(table)+` t WHERE t.tenant_id = $1`, tenantID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	w, err := zw.Create(table + ".jsonl")
	if err != nil {
		return 0, err
	}
	n := 0
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return n, err
		}
		out, err := Redact([]byte(line))
		if err != nil {
			return n, err
		}
		w.Write(out)
		w.Write([]byte("\n"))
		n++
	}
	return n, rows.Err()
}

// writeFiles adds the tenant's stored files under files/, up to the size cap.
func (m *Manager) writeFiles(ctx context.Context, zw *zip.Writer, tenantID int, man *Manifest) error {
	rows, err := m.db.QueryxContext(ctx, `SELECT "uuid"::text, filename, COALESCE("size", 0) FROM media
		WHERE tenant_id = $1 AND model_type IS DISTINCT FROM $2 ORDER BY id`, tenantID, ModelType)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			uuid, name string
			size       int64
		)
		if err := rows.Scan(&uuid, &name, &size); err != nil {
			return err
		}
		if man.FilesBytes+size > m.opts.MaxFilesBytes {
			man.FilesSkipped++
			continue
		}
		b, err := m.files.Blob(uuid)
		if err != nil {
			man.FilesSkipped++
			continue
		}
		w, err := zw.Create("files/" + uuid + "/" + safeName(name))
		if err != nil {
			return err
		}
		w.Write(b)
		man.Files++
		man.FilesBytes += int64(len(b))
	}
	return rows.Err()
}

// Redact replaces the values of credential keys in a JSON object, at any
// depth, and of settings rows whose key names a credential.
func Redact(b []byte) ([]byte, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	if row, ok := v.(map[string]any); ok {
		if k, ok := row["key"].(string); ok && sensitiveKey.MatchString(k) {
			if _, has := row["value"]; has {
				row["value"] = redacted
			}
		}
	}
	return json.Marshal(redact(v))
}

func redact(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if sensitiveKey.MatchString(k) && !isEmpty(val) {
				x[k] = redacted
				continue
			}
			x[k] = redact(val)
		}
	case []any:
		for i := range x {
			x[i] = redact(x[i])
		}
	case string:
		// Inbox and provider configs are JSON inside a text column.
		if len(x) > 1 && (x[0] == '{' || x[0] == '[') {
			var inner any
			if json.Unmarshal([]byte(x), &inner) == nil {
				b, _ := json.Marshal(redact(inner))
				return string(b)
			}
		}
	}
	return v
}

func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	}
	return false
}

func safeName(name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(name)
	if name == "" {
		return "file"
	}
	return name
}
