package ws

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/ws/models"
	"github.com/zerodha/logf"
)

// TestBroadcastStaysInTenant checks a broadcast to "everyone" only reaches the
// tenant's agents, and reaches nobody without a tenant.
func TestBroadcastStaysInTenant(t *testing.T) {
	lo := logf.New(logf.Opts{})
	h := NewHub(&lo, nil)
	client := func(id, tenantID int) *Client {
		c := &Client{ID: id, TenantID: tenantID, Hub: h, Send: make(chan models.WSMessage, 4)}
		h.AddClient(c)
		return c
	}
	acme1, acme2, globex := client(1, 2), client(2, 2), client(3, 3)

	h.BroadcastMessage(models.BroadcastMessage{Data: []byte("acme"), TenantID: 2})
	h.BroadcastMessage(models.BroadcastMessage{Data: []byte("nobody")})
	h.BroadcastMessage(models.BroadcastMessage{Data: []byte("to 3"), Users: []int{3}})

	got := func(c *Client) []string {
		var out []string
		for len(c.Send) > 0 {
			out = append(out, string((<-c.Send).Data))
		}
		return out
	}
	for _, c := range []struct {
		c    *Client
		want string
	}{{acme1, "[acme]"}, {acme2, "[acme]"}, {globex, "[to 3]"}} {
		if g := got(c.c); len(g) != 1 || "["+g[0]+"]" != c.want {
			t.Errorf("client %d (tenant %d) got %v, want %s", c.c.ID, c.c.TenantID, g, c.want)
		}
	}
}
