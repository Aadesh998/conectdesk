// Package phone normalises phone numbers, so a caller on the phone line, a
// sender on WhatsApp and a contact typed into the contact form are recognised
// as the same person.
package phone

import "strings"

// KeyLength is how many trailing digits Key keeps: a national number's length
// in India, where most contacts are, and long enough to rarely collide within
// one workspace elsewhere. Matching on it narrows the candidates, which are
// then compared in full.
const KeyLength = 10

// E164 normalises a phone number to +<digits>, taking a bare 10-digit number
// as Indian, which is how Exotel reports Indian callers (often with a 0).
func E164(phone string) string {
	d := digitsOnly(phone)
	switch {
	case d == "":
		return ""
	case len(d) == 11 && d[0] == '0':
		d = "91" + d[1:]
	case len(d) == 10:
		d = "91" + d
	}
	return "+" + d
}

// FromContact builds a contact's international number from the contact
// form's fields: the country as an ISO code (e.g. "IN"), or a calling code,
// and the number. A number already in international form stands alone.
func FromContact(country, number string) string {
	number = strings.TrimSpace(number)
	if number == "" {
		return ""
	}
	if strings.HasPrefix(number, "+") {
		return E164(number)
	}
	country = strings.TrimSpace(country)
	if cc, ok := callingCodes[strings.ToUpper(country)]; ok {
		country = cc
	}
	national := strings.TrimLeft(digitsOnly(number), "0")
	cc := strings.TrimPrefix(country, "+")
	// A number longer than any national one that starts with the calling code
	// was typed with it; otherwise it's national.
	if cc == "" || (len(national) > 10 && strings.HasPrefix(national, cc)) {
		return E164(national)
	}
	return "+" + cc + national
}

// Digits returns a number's digits in international form, for matching.
func Digits(phone string) string {
	return strings.TrimPrefix(E164(phone), "+")
}

// Key is the trailing digits of a number, as the users.phone_key column keeps
// them. It is "" for a number without digits.
func Key(phone string) string {
	d := digitsOnly(phone)
	if len(d) > KeyLength {
		d = d[len(d)-KeyLength:]
	}
	return d
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
