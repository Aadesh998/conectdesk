package main

import (
	"errors"
	"os"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/export"
	realip "github.com/ferluci/fast-realip"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// exportFiles stores exports as private media of the tenant.
type exportFiles struct{ app *App }

func (f exportFiles) Blob(name string) ([]byte, error) { return f.app.media.GetBlob(name) }

func (f exportFiles) Save(tenantID, exportID int, name string, file *os.File, size int64) (string, error) {
	m, err := f.app.media.UploadAndInsert(tenantID, name, "application/zip", "", null.StringFrom(export.ModelType),
		null.IntFrom(exportID), file, int(size), null.StringFrom("attachment"), []byte("{}"), true)
	if err != nil {
		return "", err
	}
	return m.UUID, nil
}

func (f exportFiles) Remove(tenantID int, mediaUUID string) error {
	return f.app.media.Delete(tenantID, mediaUUID)
}

// handleGetExports returns the workspace's latest data exports.
func handleGetExports(r *fastglue.Request) error {
	app := r.Context.(*App)
	list, err := app.export.List(tenantID(r))
	if err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tenantID(r)).T("globals.messages.somethingWentWrong"), nil))
	}
	return r.SendEnvelope(list)
}

// handleStartExport starts exporting all of the workspace's data.
func handleStartExport(r *fastglue.Request) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	user := r.RequestCtx.UserValue("user").(amodels.User)
	slug := "workspace"
	if t, err := app.tenant.GetCached(tid); err == nil && t.Slug != "" {
		slug = t.Slug
	}
	e, err := app.export.Start(app.ctx, tid, user.ID, user.Email, slug+"-export-"+time.Now().UTC().Format("2006-01-02")+".zip")
	if err != nil {
		if errors.Is(err, export.ErrRunning) {
			return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tid).T("export.alreadyRunning"), nil, envelope.ConflictError)
		}
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
	}
	app.activityLog.DataExported(tid, user.ID, user.Email, realip.FromRequest(r.RequestCtx))
	return r.SendEnvelope(e)
}

// handleDownloadExport streams a ready export's ZIP.
func handleDownloadExport(r *fastglue.Request) error {
	app := r.Context.(*App)
	mediaUUID, err := app.export.DownloadMedia(tenantID(r), r.RequestCtx.UserValue("uuid").(string))
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusNotFound, app.i18n.For(tenantID(r)).T("export.notReady"), nil, envelope.NotFoundError)
	}
	r.RequestCtx.QueryArgs().Set("download", "1")
	return serveMediaFile(r, app, mediaUUID, nil)
}

// exportMaxFilesBytes is how many bytes of stored files an export includes,
// app.tenancy.export_max_files_mb, 2 GB by default.
func exportMaxFilesBytes() int64 {
	mb := ko.Int64("app.tenancy.export_max_files_mb")
	if mb <= 0 {
		mb = 2048
	}
	return mb << 20
}
