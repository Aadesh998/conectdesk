package user

import (
	"crypto/rand"
	"math/big"

	"github.com/conectdesk/conectdesk/internal/envelope"
	"golang.org/x/crypto/bcrypt"
)

// Characters of generated passwords, without ones easily misread (0/O, 1/l/I).
const (
	pwUpper  = "ABCDEFGHJKLMNPQRSTUVWXYZ"
	pwLower  = "abcdefghijkmnopqrstuvwxyz"
	pwDigit  = "23456789"
	pwSymbol = "!@#$%*-_=+?"
)

// GeneratePassword returns a random 16-character password that passes
// IsStrongPassword, for a person to be sent and type in.
func GeneratePassword() (string, error) {
	const length = 16
	all := pwUpper + pwLower + pwDigit + pwSymbol
	out := make([]byte, 0, length)
	// One of each kind, then any.
	for _, set := range []string{pwUpper, pwLower, pwDigit, pwSymbol} {
		c, err := randChar(set)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	for len(out) < length {
		c, err := randChar(all)
		if err != nil {
			return "", err
		}
		out = append(out, c)
	}
	// Shuffle, so the kinds aren't always in the same places.
	for i := len(out) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		out[i], out[j.Int64()] = out[j.Int64()], out[i]
	}
	return string(out), nil
}

func randChar(set string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
	if err != nil {
		return 0, err
	}
	return set[n.Int64()], nil
}

// SetAgentPassword sets an agent's password, which must be strong.
func (u *Manager) SetAgentPassword(tenantID, id int, password string) error {
	if !IsStrongPassword(password) {
		return envelope.NewError(envelope.InputError, PasswordHint, nil)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		u.lo.Error("error generating bcrypt password", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	res, err := u.q.SetUserPassword.Exec(tenantID, hash, id)
	if err != nil {
		u.lo.Error("error setting user password", "error", err)
		return envelope.NewError(envelope.GeneralError, u.i18n.For(tenantID).T("globals.messages.somethingWentWrong"), nil)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return envelope.NewError(envelope.NotFoundError, u.i18n.For(tenantID).T("validation.notFoundUser"), nil)
	}
	return nil
}
