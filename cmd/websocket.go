package main

import (
	"fmt"
	"net/url"
	"strings"

	amodels "github.com/conectdesk/conectdesk/internal/auth/models"
	"github.com/conectdesk/conectdesk/internal/ws"
	wsmodels "github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/fasthttp/websocket"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// ErrHandler is a custom error handler.
func ErrHandler(ctx *fasthttp.RequestCtx, status int, reason error) {
	fmt.Printf("error status %d: %s", status, reason)
}

// agentUpgrader: same-origin only, with loopback allowed for dev.
var agentUpgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin:     agentOriginAllowed,
	Error:           ErrHandler,
}

// agentOriginAllowed allows the app's own origin over https, and loopback
// hosts over http for local development.
func agentOriginAllowed(ctx *fasthttp.RequestCtx) bool {
	origin := string(ctx.Request.Header.Peek("Origin"))
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	// localhost and its subdomains, like a workspace's acme.localhost, only
	// ever resolve to this machine.
	host := strings.ToLower(u.Hostname())
	isLocalhost := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if u.Scheme != "https" && !isLocalhost {
		return false
	}
	if strings.EqualFold(u.Host, string(ctx.Request.Host())) {
		return true
	}
	return isLocalhost
}

// widgetUpgrader: cross-origin by design.
var widgetUpgrader = websocket.FastHTTPUpgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(ctx *fasthttp.RequestCtx) bool {
		return true
	},
	Error: ErrHandler,
}

// handleWS handles the websocket connection.
func handleWS(r *fastglue.Request, hub *ws.Hub) error {
	var (
		auser = r.RequestCtx.UserValue("user").(amodels.User)
		app   = r.Context.(*App)
	)
	// The request context isn't valid inside the upgraded connection's handler.
	tid := tenantID(r)
	err := agentUpgrader.Upgrade(r.RequestCtx, func(conn *websocket.Conn) {
		c := ws.Client{
			ID:       auser.ID,
			TenantID: tid,
			Hub:      hub,
			Conn:     conn,
			Send:     make(chan wsmodels.WSMessage, 128),
		}
		hub.AddClient(&c)
		go c.Listen()
		c.Serve()
	})
	if err != nil {
		app.lo.Error("error upgrading tcp connection", "user_id", auser.ID, "error", err)
	}
	return nil
}
