package user

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/conectdesk/conectdesk/internal/dbutil"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/phone"
	"github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/volatiletech/null/v9"
)

// ResolveContact resolves user.ID to a contact by external_user_id or email, creating one if absent; policy decides if a matched contact may be updated.
func (u *Manager) ResolveContact(tenantID int, user *models.User, policy models.ContactPolicy) error {
	if len(user.CustomAttributes) == 0 {
		user.CustomAttributes = []byte("{}")
	}

	user.Email = null.NewString(strings.ToLower(strings.TrimSpace(user.Email.String)), user.Email.Valid)

	if policy == models.ContactSync {
		return u.syncContact(tenantID, user)
	}
	return u.reuseContact(tenantID, user)
}

// UpdateContactBasicInfo updates only the name, email and phone of a contact.
func (u *Manager) UpdateContactBasicInfo(tenantID, id int, firstName, lastName, email, phoneNumber, phoneNumberCountryCode string) error {
	if _, err := u.q.UpdateContactBasicInfo.Exec(tenantID, id, firstName, lastName, strings.ToLower(strings.TrimSpace(email)), phoneNumber, phoneNumberCountryCode); err != nil {
		u.lo.Error("error updating contact basic info", "error", err)
		return fmt.Errorf("updating contact basic info: %w", err)
	}
	return nil
}

func (u *Manager) UpdateContact(tenantID, id int, user models.User) error {
	if _, err := u.q.UpdateContact.Exec(tenantID, id, user.FirstName, user.LastName, user.Email, user.AvatarURL, user.PhoneNumber, user.PhoneNumberCountryCode, user.Country); err != nil {
		if dbutil.IsUniqueViolationError(err) {
			return envelope.NewError(envelope.InputError, u.i18n.For(tenantID).T("contact.alreadyExistsWithEmail"), nil)
		}
		u.lo.Error("error updating user", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return nil
}

// DeleteContact permanently deletes a contact or visitor; conversations, messages, and notes are removed by DB cascades.
func (u *Manager) DeleteContact(tenantID, id int) error {
	res, err := u.q.DeleteContact.Exec(tenantID, id)
	if err != nil {
		u.lo.Error("error deleting contact", "contact_id", id, "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
	}
	return nil
}

// ExportContactData returns a contact's profile, non-private conversation messages, and CSAT responses as JSON.
func (u *Manager) ExportContactData(tenantID, id int) ([]byte, error) {
	var data []byte
	if err := u.q.ExportContactData.Get(&data, tenantID, id); err != nil {
		u.lo.Error("error exporting contact data", "contact_id", id, "error", err)
		return nil, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	return data, nil
}

func (u *Manager) GetContacts(tenantID, page, pageSize int, order, orderBy string, filtersJSON, location string) ([]models.UserCompact, error) {
	if pageSize > maxListPageSize {
		pageSize = maxListPageSize
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	return u.GetAllUsers(tenantID, page, pageSize, []string{models.UserTypeContact, models.UserTypeVisitor}, order, orderBy, filtersJSON, location)
}

// reuseContact resolves to an existing contact without ever updating it, inserting one only if absent.
func (u *Manager) reuseContact(tenantID int, user *models.User) error {
	// An unknown ext_id falls back to the phone number, then to the email,
	// else a duplicate contact with the same number or email gets inserted.
	lookup := func() (models.User, error) {
		if user.ExternalUserID.String != "" {
			existing, err := u.GetContactByExternalID(tenantID, user.ExternalUserID.String)
			if !isNotFound(err) {
				return existing, err
			}
		}
		if user.PhoneNumber.String != "" {
			existing, err := u.GetContactByPhone(tenantID, user.PhoneNumber.String)
			if !isNotFound(err) {
				return existing, err
			}
		}
		if user.ExternalUserID.String != "" && user.Email.String == "" {
			return models.User{}, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
		}
		return u.GetContactByEmail(tenantID, user.Email.String)
	}

	reuse := func(existing models.User) {
		user.ID = existing.ID
		if existing.Email.String != "" {
			user.Email = existing.Email
		}
	}

	existing, err := lookup()
	if err == nil {
		reuse(existing)
		return nil
	}
	if envErr, ok := err.(envelope.Error); !ok || envErr.ErrorType != envelope.NotFoundError {
		return err
	}

	password, err := u.newContactPassword()
	if err != nil {
		return err
	}

	err = u.q.InsertContactIfAbsent.QueryRow(tenantID, user.Email, user.FirstName, user.LastName, password, user.AvatarURL, user.ExternalUserID, user.CustomAttributes).Scan(&user.ID)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		u.lo.Error("error inserting contact", "error", err)
		return fmt.Errorf("insert contact: %w", err)
	}

	// No returned row means a concurrent request inserted the contact - reuse it.
	existing, err = lookup()
	if err != nil {
		return err
	}
	reuse(existing)
	return nil
}

// syncContact resolves to an existing contact and updates its name/email, enriching external_user_id, inserting one if absent.
func (u *Manager) syncContact(tenantID int, user *models.User) error {
	// Check if email matches an existing contact without ext_id - enrich it.
	if user.ExternalUserID.String != "" {
		if user.Email.Valid && user.Email.String != "" {
			existing, emailErr := u.GetContactByEmailWithoutExtID(tenantID, user.Email.String)
			if emailErr != nil {
				if envErr, ok := emailErr.(envelope.Error); !ok || envErr.ErrorType != envelope.NotFoundError {
					return emailErr
				}
			} else {
				enriched, setErr := u.SetExternalUserID(tenantID, existing.ID, user.ExternalUserID.String)
				if setErr != nil && !dbutil.IsUniqueViolationError(setErr) {
					return setErr
				}
				if enriched {
					user.ID = existing.ID
					return nil
				}
				// ext_id already belongs to another contact, or the contact was deleted mid-flight - fall through to upsert.
				u.lo.Info("skipping contact enrichment, falling back to upsert", "contact_id", existing.ID, "ext_id", user.ExternalUserID.String)
			}
		}

		// A new ext_id from a caller or a WhatsApp sender whose number a
		// contact already has is that contact, reached on another channel.
		if matched, err := u.matchContactByPhone(tenantID, user); err != nil || matched {
			return err
		}

		password, err := u.newContactPassword()
		if err != nil {
			return err
		}

		// Upsert by ext_id - creates new or updates email/name on ext_id conflict.
		if err := u.q.InsertContactWithExtID.QueryRow(tenantID, user.Email, user.FirstName, user.LastName, password, user.AvatarURL, user.ExternalUserID, user.CustomAttributes, user.PhoneNumber, user.PhoneNumberCountryCode).Scan(&user.ID); err != nil {
			u.lo.Error("error inserting contact with external ID", "error", err)
			return fmt.Errorf("inserting contact with external ID: %w", err)
		}
		return nil
	}

	if user.Email.Valid && user.Email.String != "" {
		// An ext_id contact owns this email - reuse it; the no-ext-id upsert below can't match it and would insert a duplicate.
		existing, err := u.GetContactByEmail(tenantID, user.Email.String)
		if err == nil && existing.ExternalUserID.String != "" {
			user.ID = existing.ID
			return nil
		}

		// Other error than not found - fail.
		if err != nil {
			if envErr, ok := err.(envelope.Error); !ok || envErr.ErrorType != envelope.NotFoundError {
				return err
			}
		}
	}

	password, err := u.newContactPassword()
	if err != nil {
		return err
	}

	// No ext_id contact for this email - insert new, or update the existing no-ext-id contact's name.
	if err := u.q.InsertContactNoExtID.QueryRow(tenantID, user.Email, user.FirstName, user.LastName, password, user.AvatarURL).Scan(&user.ID); err != nil {
		u.lo.Error("error inserting contact", "error", err)
		return fmt.Errorf("insert contact: %w", err)
	}
	return nil
}

func (u *Manager) newContactPassword() ([]byte, error) {
	password, err := u.generatePassword()
	if err != nil {
		u.lo.Error("generating password", "error", err)
		return nil, fmt.Errorf("generating password: %w", err)
	}
	return password, nil
}

// GetContactByPhone returns the oldest contact whose number is number, compared
// in international form, or a NotFoundError. The phone_key index finds the
// contacts ending in the same digits and each is compared in full, since a
// contact's number may be stored national with its country apart.
func (u *Manager) GetContactByPhone(tenantID int, number string) (models.User, error) {
	want := phone.E164(number)
	key := phone.Key(want)
	// Too few digits to be a phone number anyone could be told apart by.
	if len(key) < 6 {
		return models.User{}, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
	}
	var candidates []models.User
	if err := u.q.GetContactsByPhoneKey.Select(&candidates, tenantID, key); err != nil {
		u.lo.Error("error fetching contacts by phone", "error", err)
		return models.User{}, envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	for _, c := range candidates {
		if phone.FromContact(c.PhoneNumberCountryCode.String, c.PhoneNumber.String) == want {
			return c, nil
		}
	}
	return models.User{}, envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
}

// matchContactByPhone resolves user to the contact that already has its phone
// number, when no contact has its ext_id yet: a caller on the phone line who
// then writes on WhatsApp stays one contact. A contact still named after its
// number, as an unknown caller is, takes the name the new channel gives.
func (u *Manager) matchContactByPhone(tenantID int, user *models.User) (bool, error) {
	if user.PhoneNumber.String == "" {
		return false, nil
	}
	if _, err := u.GetContactByExternalID(tenantID, user.ExternalUserID.String); !isNotFound(err) {
		return false, err
	}
	existing, err := u.GetContactByPhone(tenantID, user.PhoneNumber.String)
	if isNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	user.ID = existing.ID
	if looksLikePhone(existing.FirstName) && user.FirstName != "" && !looksLikePhone(user.FirstName) {
		if err := u.UpdateContactBasicInfo(tenantID, existing.ID, user.FirstName, user.LastName, "", "", ""); err != nil {
			u.lo.Error("error naming contact matched by phone", "contact_id", existing.ID, "error", err)
		}
	}
	return true, nil
}

// looksLikePhone reports whether a name is really a phone number, as the name
// of a caller nobody has named yet is.
func looksLikePhone(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	digits := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digits++
		case strings.ContainsRune("+-() .", r):
		default:
			return false
		}
	}
	return digits > 0
}

func isNotFound(err error) bool {
	envErr, ok := err.(envelope.Error)
	return ok && envErr.ErrorType == envelope.NotFoundError
}
