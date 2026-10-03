package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	mmodels "github.com/conectdesk/conectdesk/internal/media/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/meta"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/whatsapp"
	"github.com/zerodha/fastglue"
)

// whatsAppGraphTimeout bounds a template listing or lookup.
const whatsAppGraphTimeout = 20 * time.Second

// runningWhatsAppInbox returns a tenant's running WhatsApp inbox.
func runningWhatsAppInbox(app *App, tid, inboxID int) (*whatsapp.WhatsApp, error) {
	record, err := app.inbox.GetDBRecord(tid, inboxID)
	if err != nil {
		return nil, err
	}
	if record.Channel != inbox.ChannelWhatsApp {
		return nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("validation.notFoundInbox"), nil)
	}
	if !record.Enabled {
		return nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("status.disabledInbox"), nil)
	}
	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		return nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("status.disabledInbox"), nil)
	}
	wa, ok := inb.(*whatsapp.WhatsApp)
	if !ok {
		return nil, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
	}
	return wa, nil
}

// handleGetWhatsAppTemplates lists a WhatsApp inbox's approved templates, for
// the template picker. ?refresh=true skips the few-minute cache, for a
// template approved moments ago.
func handleGetWhatsAppTemplates(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	wa, err := runningWhatsAppInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
	defer cancel()
	templates, err := wa.Templates(c, string(r.RequestCtx.QueryArgs().Peek("refresh")) == "true")
	if err != nil {
		app.lo.Error("error listing whatsapp templates", "inbox_id", id, "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("whatsapp.templatesUnavailable"), nil))
	}

	out := make([]whatsapp.TemplateSummary, 0, len(templates))
	for _, t := range templates {
		out = append(out, whatsapp.Summarize(t))
	}
	return r.SendEnvelope(out)
}

// replyWindow is when a WhatsApp contact can be sent free-form messages.
type replyWindow struct {
	Open           bool       `json:"open"`
	LastIncomingAt *time.Time `json:"last_incoming_at"`
	ClosesAt       *time.Time `json:"closes_at"`
}

// whatsAppWindow works out the reply window for a contact in an inbox.
func whatsAppWindow(app *App, tid, contactID, inboxID int) (replyWindow, error) {
	last, err := app.conversation.LastIncomingAt(tid, contactID, inboxID)
	if err != nil {
		return replyWindow{}, err
	}
	w := replyWindow{Open: whatsapp.WindowOpen(last, time.Now())}
	if !last.IsZero() {
		closes := last.Add(whatsapp.WindowDuration)
		w.LastIncomingAt, w.ClosesAt = &last, &closes
	}
	return w, nil
}

// handleGetReplyWindow reports whether a WhatsApp conversation can take a
// free-form reply, or needs a template because the contact last wrote more
// than 24 hours ago. Other channels are always open here.
func handleGetReplyWindow(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		uuid  = r.RequestCtx.UserValue("uuid").(string)
	)
	user, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	conv, err := enforceConversationAccess(app, tid, uuid, user)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if conv.InboxChannel != inbox.ChannelWhatsApp {
		return r.SendEnvelope(replyWindow{Open: true})
	}
	w, err := whatsAppWindow(app, tid, conv.ContactID, conv.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(w)
}

// prepareWhatsAppTemplate checks an agent's template values against the
// approved template and returns the reply's content and the meta Send reads.
// media is the reply's attachments: exactly the one file for a template with
// a media header, none otherwise.
func prepareWhatsAppTemplate(app *App, tid, inboxID int, req whatsapp.TemplateRequest, media []mmodels.Media) (string, map[string]any, error) {
	wa, err := runningWhatsAppInbox(app, tid, inboxID)
	if err != nil {
		return "", nil, err
	}

	c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
	defer cancel()
	t, err := wa.FindTemplate(c, req.Name, req.Language)
	if err != nil {
		if errors.Is(err, whatsapp.ErrTemplateNotFound) {
			return "", nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("whatsapp.templateNotFound"), nil)
		}
		app.lo.Error("error fetching whatsapp template", "inbox_id", inboxID, "template", req.Name, "error", err)
		return "", nil, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("whatsapp.templatesUnavailable"), nil)
	}

	prepared, text, err := whatsapp.Prepare(t, req)
	if err != nil {
		return "", nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("whatsapp.templateInvalid", "error", err.Error()), nil)
	}
	switch {
	case prepared.HeaderMedia != "":
		if len(media) != 1 {
			return "", nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("whatsapp.templateNeedsHeaderFile"), nil)
		}
		if err := whatsapp.CheckHeaderMedia(prepared.HeaderMedia, media[0].ContentType); err != nil {
			return "", nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("whatsapp.templateInvalid", "error", err.Error()), nil)
		}
	case len(media) > 0:
		return "", nil, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("whatsapp.templateNoAttachments"), nil)
	}
	return whatsapp.RenderHTML(text), map[string]any{whatsapp.MetaTemplateKey: prepared}, nil
}

// createTemplateRequest is a new template, with its media header's sample as
// an uploaded file.
type createTemplateRequest struct {
	whatsapp.TemplateDraft
	HeaderMediaID int `json:"header_media_id"`
}

// handleListManagedTemplates lists every template on a WhatsApp inbox's
// business account with its review status, for the admin page.
func handleListManagedTemplates(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	wa, err := runningWhatsAppInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
	defer cancel()
	templates, err := wa.ListTemplates(c)
	if err != nil {
		app.lo.Error("error listing whatsapp templates", "inbox_id", id, "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("whatsapp.templatesUnavailable"), nil))
	}
	return r.SendEnvelope(templates)
}

// handleCreateWhatsAppTemplate submits a new template for Meta's review.
func handleCreateWhatsAppTemplate(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   createTemplateRequest
	)
	if err := r.Decode(&req, "json"); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("errors.parsingRequest"), nil))
	}
	wa, err := runningWhatsAppInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}

	c, cancel := context.WithTimeout(app.ctx, 2*whatsAppGraphTimeout)
	defer cancel()

	// A media header is reviewed with a sample file, uploaded to Meta first.
	switch strings.ToUpper(req.HeaderFormat) {
	case "IMAGE", "VIDEO", "DOCUMENT":
		if req.HeaderMediaID == 0 {
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).T("whatsapp.templateNeedsHeaderFile"), nil))
		}
		media, err := app.media.Get(tid, req.HeaderMediaID, "")
		if err != nil {
			return sendErrorEnvelope(r, err)
		}
		if err := whatsapp.CheckHeaderMedia(strings.ToLower(req.HeaderFormat), media.ContentType); err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("whatsapp.templateInvalid", "error", err.Error()), nil))
		}
		blob, err := app.media.GetBlob(media.UUID)
		if err != nil {
			app.lo.Error("error reading template sample", "media_id", media.ID, "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
		}
		if req.HeaderHandle, err = wa.UploadTemplateSample(c, media.Filename, media.ContentType, blob); err != nil {
			app.lo.Error("error uploading template sample", "inbox_id", id, "error", err)
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).Ts("whatsapp.templateRejected", "error", metaErrorText(err)), nil))
		}
	}

	if _, err := whatsapp.BuildTemplateComponents(req.TemplateDraft); err != nil {
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("whatsapp.templateInvalid", "error", err.Error()), nil))
	}
	tmplID, status, err := wa.CreateTemplate(c, req.TemplateDraft)
	if err != nil {
		app.lo.Error("error creating whatsapp template", "inbox_id", id, "name", req.Name, "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.InputError, app.i18n.For(tid).Ts("whatsapp.templateRejected", "error", metaErrorText(err)), nil))
	}
	app.lo.Info("whatsapp template submitted", "inbox_id", id, "name", req.Name, "status", status)
	return r.SendEnvelope(map[string]string{"id": tmplID, "status": status})
}

// handleDeleteWhatsAppTemplate deletes a template, in one language when ?id= names it.
func handleDeleteWhatsAppTemplate(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		name  = r.RequestCtx.UserValue("name").(string)
	)
	wa, err := runningWhatsAppInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
	defer cancel()
	if err := wa.DeleteTemplate(c, name, string(r.RequestCtx.QueryArgs().Peek("id"))); err != nil {
		app.lo.Error("error deleting whatsapp template", "inbox_id", id, "name", name, "error", err)
		return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).Ts("whatsapp.templateRejected", "error", metaErrorText(err)), nil))
	}
	return r.SendEnvelope(true)
}

// metaErrorText is the part of a Graph API error worth showing an admin: the
// user-facing message Meta gives for a rejected template, if any.
func metaErrorText(err error) string {
	var ge *meta.Error
	if errors.As(err, &ge) {
		if ge.UserMsg != "" {
			return ge.UserMsg
		}
		return ge.Message
	}
	return err.Error()
}

// handleGetWhatsAppHealth reports a number's standing with Meta: live from
// the Graph API, with the last alert Meta sent about it.
func handleGetWhatsAppHealth(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	wa, err := runningWhatsAppInbox(app, tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	c, cancel := context.WithTimeout(app.ctx, whatsAppGraphTimeout)
	defer cancel()

	out := map[string]any{}
	if h, err := wa.Health(c); err != nil {
		// Shown as unavailable rather than failing the panel.
		app.lo.Warn("error reading whatsapp number health", "inbox_id", id, "error", err)
		out["error"] = metaErrorText(err)
	} else {
		out["health"] = h
		out["limit"] = whatsapp.LimitLabel(h.MessagingLimitTier)
	}
	return r.SendEnvelope(out)
}
