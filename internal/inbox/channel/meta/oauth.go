package meta

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Permissions each product asks for when there is no Login configuration to
// take them from. Every one needs Advanced Access through App Review before
// accounts the app's own team doesn't manage can connect.
var (
	whatsAppScopes  = []string{"whatsapp_business_management", "whatsapp_business_messaging", "business_management"}
	messengerScopes = []string{"pages_show_list", "pages_manage_metadata", "pages_read_engagement", "pages_messaging"}
	instagramScopes = []string{"pages_show_list", "pages_manage_metadata", "pages_read_engagement", "instagram_basic", "instagram_manage_messages"}
)

// AuthorizeURL builds the Facebook Login dialog URL that connects product.
//
// WhatsApp runs Embedded Signup through its Login for Business configuration:
// the business picks or creates its WhatsApp Business Account and verifies a
// number, and the code returned is for a business token scoped to that account.
// Messenger and Instagram ask the person which Pages to share.
func (c *Client) AuthorizeURL(product, redirectURI, state string) (string, error) {
	q := url.Values{}
	q.Set("client_id", c.app.AppID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("response_type", "code")

	switch product {
	case ProductWhatsApp:
		if c.app.WhatsAppConfigID == "" {
			return "", fmt.Errorf("meta.whatsapp_config_id is not set")
		}
		q.Set("config_id", c.app.WhatsAppConfigID)
		q.Set("override_default_response_type", "true")
		q.Set("extras", `{"setup":{},"featureType":"","sessionInfoVersion":"3"}`)
	case ProductMessenger, ProductInstagram:
		if c.app.LoginConfigID != "" {
			q.Set("config_id", c.app.LoginConfigID)
			q.Set("override_default_response_type", "true")
		} else {
			scopes := messengerScopes
			if product == ProductInstagram {
				scopes = instagramScopes
			}
			q.Set("scope", strings.Join(scopes, ","))
		}
	default:
		return "", fmt.Errorf("unknown meta product %q", product)
	}

	return "https://www.facebook.com/" + c.app.version() + "/dialog/oauth?" + q.Encode(), nil
}

// Token is an access token returned by the OAuth endpoints.
type Token struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	// ExpiresIn is 0 for a token that doesn't expire, such as an Embedded
	// Signup business token or a Page token taken from a long-lived user token.
	ExpiresIn int64 `json:"expires_in"`
}

// ExchangeCode trades the code from the Login dialog for an access token.
func (c *Client) ExchangeCode(ctx context.Context, code, redirectURI string) (Token, error) {
	var t Token
	q := url.Values{}
	q.Set("client_id", c.app.AppID)
	q.Set("client_secret", c.app.AppSecret)
	q.Set("redirect_uri", redirectURI)
	q.Set("code", code)
	err := c.Get(ctx, "", "oauth/access_token", q, &t)
	return t, err
}

// LongLivedToken trades a short-lived user token for a 60-day one. Page tokens
// read with a long-lived user token don't expire, which is why the Messenger
// and Instagram flows take this step before listing Pages.
func (c *Client) LongLivedToken(ctx context.Context, shortToken string) (Token, error) {
	var t Token
	q := url.Values{}
	q.Set("grant_type", "fb_exchange_token")
	q.Set("client_id", c.app.AppID)
	q.Set("client_secret", c.app.AppSecret)
	q.Set("fb_exchange_token", shortToken)
	err := c.Get(ctx, "", "oauth/access_token", q, &t)
	return t, err
}

// Me returns the ID of the person or system user a token belongs to. It is
// stored on the inboxes they connect so a deauthorize or data deletion request
// from Meta, which names that ID, can find them.
func (c *Client) Me(ctx context.Context, token string) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	q := url.Values{"fields": {"id"}}
	if err := c.Get(ctx, token, "me", q, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// GrantedTargets returns the IDs of the objects a token was granted a
// permission on, e.g. the WhatsApp Business Accounts shared through Embedded
// Signup for whatsapp_business_management.
func (c *Client) GrantedTargets(ctx context.Context, token, scope string) ([]string, error) {
	var out struct {
		Data struct {
			GranularScopes []struct {
				Scope     string   `json:"scope"`
				TargetIDs []string `json:"target_ids"`
			} `json:"granular_scopes"`
		} `json:"data"`
	}
	q := url.Values{}
	q.Set("input_token", token)
	// debug_token is called as the app, with an app access token.
	q.Set("access_token", c.app.AppID+"|"+c.app.AppSecret)
	if err := c.Get(ctx, "", "debug_token", q, &out); err != nil {
		return nil, err
	}
	for _, s := range out.Data.GranularScopes {
		if s.Scope == scope {
			return s.TargetIDs, nil
		}
	}
	return nil, nil
}

// Page is a Facebook Page the person granted access to, with the Instagram
// professional account linked to it, if any.
type Page struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	AccessToken string `json:"access_token"`
	Instagram   *struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"instagram_business_account"`
}

// Pages lists the Pages a user token was granted, following pagination.
func (c *Client) Pages(ctx context.Context, userToken string) ([]Page, error) {
	var pages []Page
	q := url.Values{}
	q.Set("fields", "id,name,access_token,instagram_business_account{id,username,name}")
	q.Set("limit", "100")
	path := "me/accounts"

	for i := 0; i < 20; i++ {
		var out struct {
			Data   []Page `json:"data"`
			Paging struct {
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
				Next string `json:"next"`
			} `json:"paging"`
		}
		if err := c.Get(ctx, userToken, path, q, &out); err != nil {
			return nil, err
		}
		pages = append(pages, out.Data...)
		if out.Paging.Next == "" || out.Paging.Cursors.After == "" {
			break
		}
		q.Set("after", out.Paging.Cursors.After)
	}
	return pages, nil
}

// SubscribePage subscribes the app to a Page's webhook fields, without which
// Meta doesn't deliver its messages, or those of the Instagram account linked
// to it.
func (c *Client) SubscribePage(ctx context.Context, pageID, pageToken string, fields []string) error {
	form := url.Values{"subscribed_fields": {strings.Join(fields, ",")}}
	return c.PostForm(ctx, pageToken, pageID+"/subscribed_apps", form, nil)
}

// UnsubscribePage removes the app's webhook subscription from a Page.
func (c *Client) UnsubscribePage(ctx context.Context, pageID, pageToken string) error {
	return c.Delete(ctx, pageToken, pageID+"/subscribed_apps")
}

// WABA is a WhatsApp Business Account shared through Embedded Signup.
type WABA struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PhoneNumber is a number on a WhatsApp Business Account.
type PhoneNumber struct {
	ID                 string `json:"id"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	VerifiedName       string `json:"verified_name"`
	QualityRating      string `json:"quality_rating"`
	Status             string `json:"status"`
	NameStatus         string `json:"name_status"`
}

// WABAName returns a WhatsApp Business Account's name.
func (c *Client) WABAName(ctx context.Context, token, wabaID string) (string, error) {
	var w WABA
	if err := c.Get(ctx, token, wabaID, url.Values{"fields": {"id,name"}}, &w); err != nil {
		return "", err
	}
	return w.Name, nil
}

// PhoneNumbers lists the numbers on a WhatsApp Business Account.
func (c *Client) PhoneNumbers(ctx context.Context, token, wabaID string) ([]PhoneNumber, error) {
	var out struct {
		Data []PhoneNumber `json:"data"`
	}
	q := url.Values{"fields": {"id,display_phone_number,verified_name,quality_rating,status,name_status"}}
	if err := c.Get(ctx, token, wabaID+"/phone_numbers", q, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// SubscribeWABA subscribes the app to a WhatsApp Business Account's webhooks.
func (c *Client) SubscribeWABA(ctx context.Context, token, wabaID string) error {
	return c.PostForm(ctx, token, wabaID+"/subscribed_apps", url.Values{}, nil)
}

// UnsubscribeWABA removes the app's webhook subscription from a WhatsApp
// Business Account.
func (c *Client) UnsubscribeWABA(ctx context.Context, token, wabaID string) error {
	return c.Delete(ctx, token, wabaID+"/subscribed_apps")
}

// RegisterPhoneNumber registers a number for the Cloud API, which a number
// added through Embedded Signup needs before it can send. pin becomes the
// number's two-step verification PIN.
func (c *Client) RegisterPhoneNumber(ctx context.Context, token, phoneNumberID, pin string) error {
	body := map[string]string{"messaging_product": "whatsapp", "pin": pin}
	return c.PostJSON(ctx, token, phoneNumberID+"/register", body, nil)
}
