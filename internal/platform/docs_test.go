package platform

import (
	"errors"
	"testing"
)

// TestDocsViewers checks docs viewers log in apart from platform admins: a
// viewer's login opens no platform session and an admin's password doesn't
// log in to the docs, and removing a viewer ends their sessions.
func TestDocsViewers(t *testing.T) {
	m, db := newManager(t)
	if _, err := m.SetDocsViewer("not an email", "x"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid email: %v", err)
	}
	if _, err := m.SetDocsViewer(" Reader@Example.com ", "Str0ng!Passw0rd#1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetAdmin("ops@example.com", "Str0ng!Passw0rd#1"); err != nil {
		t.Fatal(err)
	}

	// An admin isn't a viewer, and a viewer isn't an admin.
	if _, _, err := m.DocsLogin("ops@example.com", "Str0ng!Passw0rd#1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("admin docs login: %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := m.Login("reader@example.com", "Str0ng!Passw0rd#1", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("viewer platform login: %v, want ErrInvalidCredentials", err)
	}
	if _, _, err := m.DocsLogin("reader@example.com", "wrong", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}

	v, token, err := m.DocsLogin("READER@example.com", "Str0ng!Passw0rd#1", "1.2.3.4")
	if err != nil || token == "" || v.Email != "reader@example.com" {
		t.Fatalf("DocsLogin: %+v %q %v", v, token, err)
	}
	if got, err := m.DocsSessionViewer(token); err != nil || got.ID != v.ID {
		t.Fatalf("DocsSessionViewer: %+v %v", got, err)
	}
	if _, err := m.SessionAdmin(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("docs token as a platform session: %v", err)
	}
	for _, bad := range []string{"", "made-up"} {
		if _, err := m.DocsSessionViewer(bad); !errors.Is(err, ErrNoSession) {
			t.Fatalf("DocsSessionViewer(%q): %v", bad, err)
		}
	}

	viewers, err := m.DocsViewers()
	if err != nil || len(viewers) != 1 || !viewers[0].LastLoginAt.Valid {
		t.Fatalf("DocsViewers: %+v %v", viewers, err)
	}

	m.DocsLogout(token)
	if _, err := m.DocsSessionViewer(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("after logout: %v", err)
	}

	_, token, _ = m.DocsLogin("reader@example.com", "Str0ng!Passw0rd#1", "")
	db.MustExec(`UPDATE platform_docs_sessions SET expires_at = NOW() - INTERVAL '1 second'`)
	if _, err := m.DocsSessionViewer(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("expired session: %v", err)
	}

	_, token, _ = m.DocsLogin("reader@example.com", "Str0ng!Passw0rd#1", "")
	if ok, err := m.RemoveDocsViewer("reader@example.com"); !ok || err != nil {
		t.Fatalf("RemoveDocsViewer: %v %v", ok, err)
	}
	if _, err := m.DocsSessionViewer(token); !errors.Is(err, ErrNoSession) {
		t.Fatalf("session of a removed viewer: %v", err)
	}
	if ok, _ := m.RemoveDocsViewer("reader@example.com"); ok {
		t.Fatal("removed a viewer twice")
	}
}
