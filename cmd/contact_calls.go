package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/inbox/channel/voice"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	umodels "github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/valyala/fasthttp"
	"github.com/volatiletech/null/v9"
	"github.com/zerodha/fastglue"
)

// importContacts is the importer's name for contact imports.
const importContacts = "contacts"

// callContactRequest picks the voice inbox a call to a contact goes out on.
type callContactRequest struct {
	InboxID int `json:"inbox_id"`
}

// handleCallContact calls a contact from the contacts list: the call is
// logged on their open conversation in the voice inbox, or on a new one
// assigned to the calling agent, and Exotel rings the agent's phone first.
func handleCallContact(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		id, _ = strconv.Atoi(r.RequestCtx.UserValue("id").(string))
		req   callContactRequest
	)
	if err := r.Decode(&req, "json"); err != nil || id <= 0 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	if voice.E164(agent.PhoneNumber.String) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("voice.agentNoPhone"), nil, envelope.InputError)
	}
	contact, err := app.user.GetContactOrVisitor(tid, id, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	cuuid, err := callContact(app, tid, agent, contact, req.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"conversation_uuid": cuuid})
}

// dialRequest is a number typed into the dialer.
type dialRequest struct {
	Phone   string `json:"phone"`
	Name    string `json:"name"`
	InboxID int    `json:"inbox_id"`
	// ContactID calls a known contact on the number typed.
	ContactID int `json:"contact_id"`
}

// handleDial calls a number typed into the dialer. The caller ID is the
// voice inbox's ExoPhone and Exotel rings the agent's own phone first. The
// number becomes a contact, or is matched to the one it already is.
func handleDial(r *fastglue.Request) error {
	var (
		app   = r.Context.(*App)
		tid   = tenantID(r)
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		req   dialRequest
	)
	if err := r.Decode(&req, "json"); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("errors.parsingRequest"), nil, envelope.InputError)
	}
	agent, err := app.user.GetAgentCachedOrLoad(tid, auser.ID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	phone := dialNumber(req.Phone)
	if phone == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("voice.invalidDialNumber"), nil, envelope.InputError)
	}
	if voice.E164(agent.PhoneNumber.String) == "" {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("voice.agentNoPhone"), nil, envelope.InputError)
	}
	if _, _, err := runningVoiceInbox(app, tid, req.InboxID); err != nil {
		return sendErrorEnvelope(r, err)
	}

	var contact umodels.User
	if req.ContactID > 0 {
		if contact, err = app.user.GetContactOrVisitor(tid, req.ContactID, ""); err != nil {
			return sendErrorEnvelope(r, err)
		}
	} else {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = phone
		}
		first, last, _ := strings.Cut(name, " ")
		contact = umodels.User{
			FirstName:        first,
			LastName:         strings.TrimSpace(last),
			PhoneNumber:      null.StringFrom(phone),
			ExternalUserID:   null.StringFrom(voice.ExternalIDPrefix + voice.Digits(phone)),
			CustomAttributes: json.RawMessage(`{}`),
		}
		// An existing contact keeps its details; a new one gets the number.
		if err := app.user.ResolveContact(tid, &contact, umodels.ContactReuse); err != nil {
			return sendErrorEnvelope(r, envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil))
		}
	}
	if err := app.user.UpdateContactBasicInfo(tid, contact.ID, "", "", "", phone, ""); err != nil {
		app.lo.Error("error saving dialed number on contact", "contact_id", contact.ID, "error", err)
	}
	saved, err := app.user.GetContactOrVisitor(tid, contact.ID, "")
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	cuuid, err := callContact(app, tid, agent, saved, req.InboxID)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(map[string]any{"conversation_uuid": cuuid, "contact_id": saved.ID})
}

// dialNumber is a typed number with its country code, e.g. +919876543210,
// or "" if it isn't one. A 10-digit number is taken as Indian, as Exotel's
// numbers are.
func dialNumber(typed string) string {
	typed = strings.TrimSpace(typed)
	if strings.HasPrefix(typed, "00") {
		typed = "+" + strings.TrimLeft(typed, "0")
	}
	if strings.Trim(typed, "+0123456789 ()-") != "" {
		return ""
	}
	phone := voice.E164(typed)
	if !strings.HasPrefix(typed, "+") && len(voice.Digits(typed)) == 10 {
		phone = voice.E164("+91" + voice.Digits(typed))
	}
	if d := len(voice.Digits(phone)); d < 10 || d > 15 {
		return ""
	}
	return phone
}

// callContact calls a contact from a voice inbox. The call is logged on
// their open conversation in the inbox if the agent can see it, else on a
// new one assigned to the agent, which is removed again if the call can't
// be placed. It returns the conversation's UUID.
func callContact(app *App, tid int, agent umodels.User, contact umodels.User, inboxID int) (string, error) {
	phone := voice.ContactE164(contact.PhoneNumberCountryCode.String, contact.PhoneNumber.String)
	if len(phone) < 9 {
		return "", envelope.NewError(envelope.InputError, app.i18n.For(tid).T("voice.contactNoPhone"), nil)
	}
	if _, _, err := runningVoiceInbox(app, tid, inboxID); err != nil {
		return "", err
	}

	cuuid, err := app.conversation.LatestContactConversation(tid, contact.ID, inboxID)
	if err != nil {
		return "", envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
	}
	if cuuid != "" {
		if _, err := enforceConversationAccess(app, tid, cuuid, agent); err != nil {
			cuuid = ""
		}
	}
	created := false
	if cuuid == "" {
		name := strings.TrimSpace(contact.FirstName + " " + contact.LastName)
		if name == "" {
			name = phone
		}
		_, cuuid, err = app.conversation.CreateConversation(tid, contact.ID, inboxID, "", time.Now(),
			"Call: "+name, true, nil, nil, 0, 0)
		if err != nil {
			app.lo.Error("error creating conversation for a call", "contact_id", contact.ID, "error", err)
			return "", envelope.NewError(envelope.GeneralError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil)
		}
		created = true
		if err := app.conversation.UpdateConversationUserAssignee(tid, cuuid, agent.ID, agent); err != nil {
			app.lo.Error("error assigning call conversation", "conversation_uuid", cuuid, "error", err)
		}
	}

	if _, err := placeCall(app, tid, agent, cuuid, phone, inboxID); err != nil {
		if created {
			if err := app.conversation.DeleteConversation(tid, cuuid); err != nil {
				app.lo.Error("error deleting conversation", "error", err)
			}
		}
		return "", err
	}
	return cuuid, nil
}

// handleImportContacts imports contacts from a CSV: one per row, with
// first_name and an email or a phone number. See processContactImport.
func handleImportContacts(r *fastglue.Request) error {
	var (
		app = r.Context.(*App)
		tid = tenantID(r)
	)
	file, err := r.RequestCtx.FormFile("file")
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).Ts("globals.messages.required", "name", "{globals.terms.file}"), nil, envelope.InputError)
	}
	f, err := file.Open()
	if err != nil {
		app.lo.Error("error opening uploaded file", "error", err)
		return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("validation.invalidCsvFile"), nil, envelope.InputError)
	}
	if len(records) < 2 {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("importer.csvMustContainHeadersAndData"), nil, envelope.InputError)
	}
	header := contactImportHeader(records[0])
	if _, ok := header["first_name"]; !ok {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("contact.import.needsColumns"), nil, envelope.InputError)
	}
	_, hasEmail := header["email"]
	_, hasPhone := header["phone_number"]
	if !hasEmail && !hasPhone {
		return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("contact.import.needsColumns"), nil, envelope.InputError)
	}

	if err := app.importer.Submit(tid, importContacts, func() error {
		return processContactImport(app, tid, records)
	}); err != nil {
		return r.SendErrorEnvelope(fasthttp.StatusConflict, app.i18n.For(tid).T("importer.importAlreadyInProgress"), nil, envelope.GeneralError)
	}
	return r.SendEnvelope(true)
}

// handleGetContactImportStatus returns the progress of a contact import.
func handleGetContactImportStatus(r *fastglue.Request) error {
	app := r.Context.(*App)
	status, err := app.importer.GetStatus(tenantID(r), importContacts)
	if err != nil {
		return sendErrorEnvelope(r, err)
	}
	return r.SendEnvelope(status)
}

// contactImportHeader maps a CSV's column names to their positions, taking
// the common spellings of each ("Phone", "Mobile", "First Name").
func contactImportHeader(row []string) map[string]int {
	aliases := map[string]string{
		"first_name": "first_name", "firstname": "first_name", "first": "first_name", "name": "first_name",
		"last_name": "last_name", "lastname": "last_name", "last": "last_name", "surname": "last_name",
		"email": "email", "email_address": "email", "e-mail": "email",
		"phone_number": "phone_number", "phone": "phone_number", "mobile": "phone_number", "mobile_number": "phone_number", "number": "phone_number",
		"country_code": "country_code", "country": "country_code",
		"external_user_id": "external_user_id",
	}
	out := map[string]int{}
	for i, h := range row {
		key := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		key = strings.ReplaceAll(key, " ", "_")
		if col, ok := aliases[key]; ok {
			if _, dup := out[col]; !dup {
				out[col] = i
			}
		}
	}
	return out
}

// contactImportRow is one CSV row, checked and normalised.
type contactImportRow struct {
	FirstName, LastName, Email, ExternalUserID string
	// Phone is the number with its country code, e.g. +919876543210, and
	// Country the ISO code it was given with, if any.
	Phone, Country string
}

// parseContactImportRow checks one row: it needs a first name and an email
// or a phone number. A phone number without a leading + takes its country
// from country_code: an ISO code like IN, or a calling code like 91.
func parseContactImportRow(record []string, header map[string]int) (contactImportRow, string) {
	row := contactImportRow{
		FirstName:      getField(record, header, "first_name"),
		LastName:       getField(record, header, "last_name"),
		Email:          strings.ToLower(getField(record, header, "email")),
		ExternalUserID: getField(record, header, "external_user_id"),
	}
	phone := getField(record, header, "phone_number")
	country := strings.ToUpper(strings.TrimPrefix(getField(record, header, "country_code"), "+"))

	if row.FirstName == "" {
		return row, "first_name is empty"
	}
	if row.Email == "" && phone == "" {
		return row, "needs an email or a phone_number"
	}
	if row.Email != "" && !stringutil.ValidEmail(row.Email) {
		return row, fmt.Sprintf("invalid email %q", row.Email)
	}
	if phone != "" {
		switch {
		case strings.HasPrefix(phone, "+"), strings.HasPrefix(phone, "00"):
			row.Phone = voice.E164("+" + strings.TrimLeft(strings.TrimPrefix(phone, "+"), "0"))
		case country != "" && strings.Trim(country, "0123456789") == "":
			// A calling code, e.g. 91.
			row.Phone = voice.E164("+" + country + strings.TrimLeft(onlyDigits(phone), "0"))
		case country != "":
			row.Phone = voice.ContactE164(country, phone)
			row.Country = country
		default:
			return row, fmt.Sprintf("phone number %q needs a country code: start it with +, e.g. +91 98765 43210, or fill country_code", phone)
		}
		if d := len(voice.Digits(row.Phone)); d < 8 || d > 15 {
			return row, fmt.Sprintf("invalid phone number %q", phone)
		}
	}
	return row, ""
}

// processContactImport creates or updates a contact per CSV row. Contacts
// are matched on external_user_id, else their email, else their phone
// number, the same key an incoming call from them uses, so importing a list
// twice updates it rather than duplicating it.
func processContactImport(app *App, tid int, records [][]string) error {
	header := contactImportHeader(records[0])
	lang := app.i18n.For(tid)
	total := len(records) - 1
	app.importer.UpdateCounts(tid, importContacts, total, 0, 0)
	app.importer.AddLog(tid, importContacts, lang.Ts("importer.startingImport", "count", strconv.Itoa(total), "type", "contacts"))

	for i, record := range records[1:] {
		rowNum := strconv.Itoa(i + 2) // As a spreadsheet numbers it, after the header.
		row, problem := parseContactImportRow(record, header)
		if problem != "" {
			app.importer.UpdateCounts(tid, importContacts, 0, 0, 1)
			app.importer.AddLog(tid, importContacts, fmt.Sprintf("Row %s: %s", rowNum, problem))
			continue
		}

		contact := umodels.User{
			FirstName:        row.FirstName,
			LastName:         row.LastName,
			Email:            null.NewString(row.Email, row.Email != ""),
			ExternalUserID:   null.NewString(row.ExternalUserID, row.ExternalUserID != ""),
			PhoneNumber:      null.NewString(row.Phone, row.Phone != ""),
			CustomAttributes: json.RawMessage(`{}`),
		}
		if row.Country != "" {
			contact.PhoneNumberCountryCode = null.StringFrom(row.Country)
		}
		// A contact known only by number is keyed on it, as a caller is.
		if row.ExternalUserID == "" && row.Email == "" {
			contact.ExternalUserID = null.StringFrom(voice.ExternalIDPrefix + voice.Digits(row.Phone))
		}
		if err := app.user.ResolveContact(tid, &contact, umodels.ContactSync); err != nil {
			app.importer.UpdateCounts(tid, importContacts, 0, 0, 1)
			app.importer.AddLog(tid, importContacts, fmt.Sprintf("Row %s: couldn't save %s: %v", rowNum, row.FirstName, err))
			continue
		}
		// A match on email keeps its old details; bring them up to date.
		if err := app.user.UpdateContactBasicInfo(tid, contact.ID, row.FirstName, row.LastName, row.Email, row.Phone, row.Country); err != nil {
			app.importer.UpdateCounts(tid, importContacts, 0, 0, 1)
			app.importer.AddLog(tid, importContacts, fmt.Sprintf("Row %s: couldn't save %s: %v", rowNum, row.FirstName, err))
			continue
		}
		app.importer.UpdateCounts(tid, importContacts, 0, 1, 0)
		app.importer.AddLog(tid, importContacts, fmt.Sprintf("Row %s: imported %s %s", rowNum, row.FirstName, strings.TrimSpace(row.Phone+" "+row.Email)))
	}

	status, _ := app.importer.GetStatus(tid, importContacts)
	app.importer.AddLog(tid, importContacts, lang.Ts("importer.importComplete",
		"success", strconv.Itoa(status.Success), "total", strconv.Itoa(status.Total), "errors", strconv.Itoa(status.Errors)))
	return nil
}

// onlyDigits is s with everything but its digits dropped.
func onlyDigits(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}
