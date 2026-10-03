package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/crypto"
	"github.com/conectdesk/conectdesk/internal/envelope"
	"github.com/conectdesk/conectdesk/internal/sealed"
	"github.com/conectdesk/conectdesk/internal/stringutil"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// sealedKeyTTL is how long the browser has to use a key it was given: long
// enough to find and paste credentials from the Meta dashboard.
const sealedKeyTTL = 15 * time.Minute

// Purposes a sealed payload is bound to. An envelope sealed for one is refused
// by every other.
const (
	sealedPurposeWhatsAppManual = "whatsapp_manual"
)

// sealedKeyRecord is a one-time key kept until its envelope arrives. The
// private key is encrypted with the app's encryption key, so Redis alone
// can't open anything.
type sealedKeyRecord struct {
	Private  string `json:"private"`
	Purpose  string `json:"purpose"`
	TenantID int    `json:"tenant_id"`
	UserID   int    `json:"user_id"`
}

func sealedRedisKey(id string) string {
	return "sealed_key:" + id
}

// issueSealedKey returns a one-time public key the browser encrypts a payload
// for purpose to. It is bound to the workspace and admin who asked.
func issueSealedKey(r *fastglue.Request, purpose string) error {
	var (
		app     = r.Context.(*App)
		tid     = tenantID(r)
		user, _ = r.RequestCtx.UserValue("user").(amodels.User)
		failed  = func(msg string, err error) error {
			app.lo.Error(msg, "error", err)
			return r.SendErrorEnvelope(fasthttp.StatusInternalServerError, app.i18n.For(tid).T("globals.messages.somethingWentWrong"), nil, envelope.GeneralError)
		}
	)

	id, err := stringutil.RandomAlphanumeric(32)
	if err != nil {
		return failed("error generating sealed key id", err)
	}
	key, err := sealed.NewKey(id)
	if err != nil {
		return failed("error generating sealed key", err)
	}
	private, err := crypto.Encrypt(base64.StdEncoding.EncodeToString(key.PrivateBytes()), ko.MustString("app.encryption_key"))
	if err != nil {
		return failed("error encrypting sealed key", err)
	}
	rec, _ := json.Marshal(sealedKeyRecord{Private: private, Purpose: purpose, TenantID: tid, UserID: user.ID})
	if err := app.redis.Set(ctx, sealedRedisKey(id), rec, sealedKeyTTL).Err(); err != nil {
		return failed("error storing sealed key", err)
	}

	return r.SendEnvelope(map[string]any{
		"key_id":     id,
		"public_key": key.PublicKey(),
		"algorithm":  sealed.Algorithm,
		"info":       sealed.Info,
		"expires_in": int(sealedKeyTTL.Seconds()),
	})
}

// errSealedExpired is returned for an envelope whose key was used, expired, or
// was issued to someone else, so the browser should fetch a new key.
var errSealedExpired = errors.New("sealed key expired or already used")

// openSealed decrypts an envelope sealed for purpose, deleting its key as it
// does: each key opens one envelope, once, for the admin it was issued to.
func openSealed(r *fastglue.Request, env sealed.Envelope, purpose string) ([]byte, error) {
	var (
		app     = r.Context.(*App)
		tid     = tenantID(r)
		user, _ = r.RequestCtx.UserValue("user").(amodels.User)
	)
	if env.KeyID == "" {
		return nil, errSealedExpired
	}
	raw, err := app.redis.GetDel(ctx, sealedRedisKey(env.KeyID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, errSealedExpired
	} else if err != nil {
		return nil, err
	}

	var rec sealedKeyRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return nil, err
	}
	if rec.Purpose != purpose || rec.TenantID != tid || rec.UserID != user.ID {
		app.lo.Warn("sealed key presented by someone it wasn't issued to", "tenant_id", tid, "user_id", user.ID)
		return nil, errSealedExpired
	}
	b64, err := crypto.Decrypt(rec.Private, ko.MustString("app.encryption_key"))
	if err != nil {
		return nil, err
	}
	private, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	key, err := sealed.ParseKey(env.KeyID, private)
	if err != nil {
		return nil, err
	}
	return key.Open(env, purpose)
}

// sendSealedError answers a payload that couldn't be opened, telling the
// browser to start over with a new key.
func sendSealedError(r *fastglue.Request, err error) error {
	app := r.Context.(*App)
	tid := tenantID(r)
	if !errors.Is(err, errSealedExpired) && !errors.Is(err, sealed.ErrOpen) {
		app.lo.Error("error opening sealed payload", "error", err)
	}
	return r.SendErrorEnvelope(fasthttp.StatusBadRequest, app.i18n.For(tid).T("sealed.failed"), nil, envelope.InputError)
}
