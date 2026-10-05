package platform

import (
	"database/sql"
	"errors"
	"time"

	"github.com/conectdesk/conectdesk/internal/platform/models"
	"golang.org/x/crypto/bcrypt"
)

// DocsSessionTTL is how long a docs viewer's login lasts.
const DocsSessionTTL = 7 * 24 * time.Hour

// SetDocsViewer creates a docs viewer, or sets the password of an existing
// one. The caller checks the password is strong enough.
func (m *Manager) SetDocsViewer(email, password string) (models.DocsViewer, error) {
	var v models.DocsViewer
	email, err := normalizeEmail(email)
	if err != nil {
		return v, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return v, err
	}
	if err := m.q.UpsertDocsViewer.Get(&v, email, string(hash)); err != nil {
		m.lo.Error("error saving docs viewer", "error", err)
		return v, err
	}
	return v, nil
}

// DocsViewers returns every docs viewer.
func (m *Manager) DocsViewers() ([]models.DocsViewer, error) {
	out := []models.DocsViewer{}
	if err := m.q.GetDocsViewers.Select(&out); err != nil {
		m.lo.Error("error fetching docs viewers", "error", err)
		return nil, err
	}
	return out, nil
}

// RemoveDocsViewer removes a docs viewer and ends their sessions.
func (m *Manager) RemoveDocsViewer(email string) (bool, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return false, err
	}
	res, err := m.q.DeleteDocsViewer.Exec(email)
	if err != nil {
		m.lo.Error("error removing docs viewer", "error", err)
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DocsLogin checks the viewer's password and starts a session, returning its
// token for the session cookie.
func (m *Manager) DocsLogin(email, password, ip string) (models.DocsViewer, string, error) {
	var v models.DocsViewer
	email, _ = normalizeEmail(email)
	err := m.q.GetDocsViewerByEmail.Get(&v, email)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return v, "", ErrInvalidCredentials
	case err != nil:
		m.lo.Error("error fetching docs viewer", "error", err)
		return v, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(v.PasswordHash), []byte(password)) != nil {
		return v, "", ErrInvalidCredentials
	}
	token, hash, err := newToken()
	if err != nil {
		return v, "", err
	}
	m.q.DeleteExpiredDocsSessions.Exec()
	if _, err := m.q.InsertDocsSession.Exec(v.ID, hash, time.Now().Add(DocsSessionTTL), ip); err != nil {
		m.lo.Error("error saving docs session", "error", err)
		return v, "", err
	}
	m.q.TouchDocsViewerLogin.Exec(v.ID)
	return v, token, nil
}

// DocsSessionViewer returns the docs viewer a session token belongs to.
func (m *Manager) DocsSessionViewer(token string) (models.DocsViewer, error) {
	var v models.DocsViewer
	if token == "" {
		return v, ErrNoSession
	}
	if err := m.q.GetDocsSessionViewer.Get(&v, hashToken(token)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return v, ErrNoSession
		}
		m.lo.Error("error fetching docs session", "error", err)
		return v, err
	}
	return v, nil
}

// DocsLogout ends a docs session.
func (m *Manager) DocsLogout(token string) error {
	_, err := m.q.DeleteDocsSession.Exec(hashToken(token))
	return err
}
