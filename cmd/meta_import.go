package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/messenger"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// metaImportTimeout bounds a whole history import.
const metaImportTimeout = 30 * time.Minute

// metaImportRequest is the body of a history import.
type metaImportRequest struct {
	messenger.ImportOpts
	// Resolve files the imported conversations as resolved, so old history
	// doesn't fill the open queue. On unless turned off.
	Resolve *bool `json:"resolve"`
}

func metaImportNamespace(inboxID int) string {
	return "meta_history:" + strconv.Itoa(inboxID)
}

// handleImportMetaHistory starts importing a Messenger or Instagram inbox's
// past conversations in the background. WhatsApp has no such API: its
// history stays on the phone.
func handleImportMetaHistory(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   metaImportRequest
	)
	if len(r.RequestCtx.PostBody()) > 0 {
		if err := r.Decode(&req, "json"); err != nil {
			return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
		}
	}
	resolve := req.Resolve == nil || *req.Resolve

	record, err := app.inbox.GetDBRecord(tid, id)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if record.Channel != inbox.ChannelMessenger && record.Channel != inbox.ChannelInstagram {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("inbox.historyUnsupported"), nil, envelope.InputError)
	}
	inb, err := app.inbox.Get(record.ID)
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("status.disabledInbox"), nil, envelope.InputError)
	}
	m, ok := inb.(*messenger.Messenger)
	if !ok {
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}

	ns := metaImportNamespace(id)
	err = app.importer.Submit(tid, ns, func() error {
		c, cancel := context.WithTimeout(app.ctx, metaImportTimeout)
		defer cancel()

		imported := 0
		seen, errs, err := m.History(c, req.ImportOpts, func(chat messenger.Chat) error {
			n, err := app.conversation.ImportChat(tid, id, chat.Contact, chat.Messages, resolve)
			if err != nil {
				app.importer.UpdateCounts(tid, ns, 0, 0, 1)
				return err
			}
			imported += n
			app.importer.UpdateCounts(tid, ns, 0, 1, 0)
			return nil
		})
		app.importer.UpdateCounts(tid, ns, seen, 0, 0)
		for _, e := range errs {
			app.importer.AddLog(tid, ns, e)
		}
		app.importer.AddLog(tid, ns, app.i18n.For(tid).Ts("inbox.historyImported",
			"messages", strconv.Itoa(imported), "conversations", strconv.Itoa(seen)))
		if err != nil {
			app.importer.AddLog(tid, ns, err.Error())
			return fmt.Errorf("importing %s history: %w", record.Channel, err)
		}
		return nil
	})
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tid).T("importer.importAlreadyInProgress"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}

// handleGetMetaHistoryImport reports a history import's progress.
func handleGetMetaHistoryImport(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
	)
	status, err := app.importer.GetStatus(tenantID(r), metaImportNamespace(id))
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(status)
}
