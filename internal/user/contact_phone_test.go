package user

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/user/models"
	"github.com/volatiletech/null/v9"
)

func phoneContact(extID, number, firstName string) *models.User {
	c := newContact("", extID, firstName, "")
	c.PhoneNumber = null.StringFrom(number)
	return c
}

// TestResolveContactByPhone checks a caller and a WhatsApp sender with the same
// number resolve to one contact, whichever channel came first, that the
// caller's placeholder name gives way to the WhatsApp profile name, and that a
// different number, or one typed in national form on the contact form, is
// told apart correctly.
func TestResolveContactByPhone(t *testing.T) {
	u, db := newTestManager(t)

	// An unknown caller, named after their number, as the phone line logs them.
	caller := phoneContact("voice:919876543210", "+919876543210", "+919876543210")
	resolve(t, u, caller, models.ContactSync)

	// The same person writes on WhatsApp.
	sender := phoneContact("whatsapp:919876543210", "919876543210", "Asha")
	resolve(t, u, sender, models.ContactSync)
	if sender.ID != caller.ID {
		t.Fatalf("WhatsApp sender got contact %d, want the caller's %d", sender.ID, caller.ID)
	}
	if r := fetchRow(t, db, caller.ID); r.FirstName != "Asha" || r.ExtID != "voice:919876543210" {
		t.Fatalf("contact after the WhatsApp message = %+v", r)
	}

	// Their next WhatsApp message, and their next call, land on it too.
	again := phoneContact("whatsapp:919876543210", "919876543210", "Asha K")
	resolve(t, u, again, models.ContactSync)
	call := phoneContact("voice:919876543210", "+919876543210", "+919876543210")
	resolve(t, u, call, models.ContactSync)
	if again.ID != caller.ID || call.ID != caller.ID {
		t.Fatalf("later messages went to %d and %d, want %d", again.ID, call.ID, caller.ID)
	}
	// A real name isn't overwritten by a placeholder.
	if r := fetchRow(t, db, caller.ID); r.FirstName != "Asha" {
		t.Fatalf("name became %q", r.FirstName)
	}

	// Another number is another contact.
	other := phoneContact("whatsapp:919876543211", "919876543211", "Ravi")
	resolve(t, u, other, models.ContactSync)
	if other.ID == caller.ID {
		t.Fatal("a different number matched the contact")
	}

	// A contact typed with a national number and a country code is matched by
	// a caller in international form, and reused by an agent starting a chat.
	var typed int
	db.QueryRow(`INSERT INTO users (tenant_id, type, first_name, phone_number, phone_number_country_code)
		VALUES (1, 'contact', 'Meera', '09123456789', 'IN') RETURNING id`).Scan(&typed)
	meera := phoneContact("voice:919123456789", "+919123456789", "+919123456789")
	resolve(t, u, meera, models.ContactSync)
	if meera.ID != typed {
		t.Fatalf("caller got contact %d, want the typed contact %d", meera.ID, typed)
	}
	reused := phoneContact("whatsapp:919123456789", "919123456789", "Meera")
	resolve(t, u, reused, models.ContactReuse)
	if reused.ID != typed {
		t.Fatalf("reused contact %d, want %d", reused.ID, typed)
	}

	// Same trailing digits, different country: not the same person.
	uk := phoneContact("whatsapp:449123456789", "449123456789", "Tom")
	resolve(t, u, uk, models.ContactSync)
	if uk.ID == typed {
		t.Fatal("a number in another country matched on its trailing digits")
	}
}

func TestLooksLikePhone(t *testing.T) {
	for s, want := range map[string]bool{
		"+91 98765-43210": true,
		"(415) 555 0100":  true,
		"":                true,
		"Asha":            false,
		"Asha 2":          false,
		"+":               false,
	} {
		if got := looksLikePhone(s); got != want {
			t.Errorf("looksLikePhone(%q) = %v, want %v", s, got, want)
		}
	}
}
