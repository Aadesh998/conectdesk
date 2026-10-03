package teams

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// DefaultOpenIDURL describes the keys the Bot Framework signs requests with.
	DefaultOpenIDURL = "https://login.botframework.com/v1/.well-known/openidconfiguration"
	// botIssuer is the issuer of the tokens the Bot Framework sends the bot.
	botIssuer = "https://api.botframework.com"

	keysTTL = 24 * time.Hour
	// minRefresh stops an unknown key ID from fetching the keys on every request.
	minRefresh = 5 * time.Minute
)

// ErrUnauthorized is returned for a request the Bot Framework didn't sign for this bot.
var ErrUnauthorized = errors.New("request not signed by the bot framework for this bot")

// Verifier checks the token the Bot Framework sends with each request.
type Verifier struct {
	appID     string
	openIDURL string
	http      *http.Client

	mu        sync.Mutex
	keys      map[string]signingKey
	fetchedAt time.Time
}

type signingKey struct {
	key *rsa.PublicKey
	// endorsements are the channels, such as msteams, the key may sign for.
	endorsements []string
}

// NewVerifier returns a verifier for the bot. openIDURL is DefaultOpenIDURL outside tests.
func NewVerifier(appID string, httpClient *http.Client, openIDURL string) *Verifier {
	if openIDURL == "" {
		openIDURL = DefaultOpenIDURL
	}
	return &Verifier{appID: appID, openIDURL: openIDURL, http: httpClient}
}

// Verify checks the Authorization header of a request carrying act: a token
// signed with one of the Bot Framework's keys, issued to this bot, not
// expired, and naming the service URL the activity asks to be answered on.
func (v *Verifier) Verify(ctx context.Context, authHeader string, act Activity) error {
	raw, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || raw == "" || v.appID == "" {
		return ErrUnauthorized
	}

	var endorsements []string
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		k, err := v.key(ctx, kid)
		if err != nil {
			return nil, err
		}
		endorsements = k.endorsements
		return k.key, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(botIssuer),
		jwt.WithAudience(v.appID),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(5*time.Minute),
	)
	if err != nil || !tok.Valid {
		return fmt.Errorf("%w: %v", ErrUnauthorized, err)
	}
	if len(endorsements) > 0 && act.ChannelID != "" && !slices.Contains(endorsements, act.ChannelID) {
		return fmt.Errorf("%w: key not endorsed for %s", ErrUnauthorized, act.ChannelID)
	}
	claims, _ := tok.Claims.(jwt.MapClaims)
	if su, _ := claims["serviceurl"].(string); su != "" && !strings.EqualFold(strings.TrimRight(su, "/"), strings.TrimRight(act.ServiceURL, "/")) {
		return fmt.Errorf("%w: token is for another service url", ErrUnauthorized)
	}
	return nil
}

// key returns the signing key with the ID, fetching the keys when they are
// stale or the ID is new, as Microsoft rolls its keys.
func (v *Verifier) key(ctx context.Context, kid string) (signingKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok && time.Since(v.fetchedAt) < keysTTL {
		return k, nil
	}
	if time.Since(v.fetchedAt) > minRefresh || v.keys == nil {
		keys, err := v.fetchKeys(ctx)
		if err != nil {
			if k, ok := v.keys[kid]; ok {
				return k, nil
			}
			return signingKey{}, err
		}
		v.keys, v.fetchedAt = keys, time.Now()
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return signingKey{}, fmt.Errorf("unknown signing key %q", kid)
}

func (v *Verifier) fetchKeys(ctx context.Context) (map[string]signingKey, error) {
	var cfg struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.getJSON(ctx, v.openIDURL, &cfg); err != nil {
		return nil, err
	}
	if cfg.JWKSURI == "" {
		return nil, errors.New("bot framework openid configuration has no jwks_uri")
	}
	var set struct {
		Keys []struct {
			Kty          string   `json:"kty"`
			Kid          string   `json:"kid"`
			N            string   `json:"n"`
			E            string   `json:"e"`
			Endorsements []string `json:"endorsements"`
		} `json:"keys"`
	}
	if err := v.getJSON(ctx, cfg.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := make(map[string]signingKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		keys[k.Kid] = signingKey{
			key:          &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())},
			endorsements: k.Endorsements,
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("bot framework published no usable signing keys")
	}
	return keys, nil
}

func (v *Verifier) getJSON(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s answered %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
