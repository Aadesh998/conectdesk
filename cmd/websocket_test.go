package main

import (
	"testing"

	"github.com/valyala/fasthttp"
)

func TestAgentOriginAllowed(t *testing.T) {
	for _, c := range []struct {
		host, origin string
		want         bool
	}{
		{"acme.yourapp.com", "https://acme.yourapp.com", true},
		{"acme.yourapp.com", "http://acme.yourapp.com", false},
		{"acme.yourapp.com", "https://evil.com", false},
		{"acme.localhost:9000", "http://acme.localhost:9000", true},
		{"localhost:9000", "http://localhost:9000", true},
		{"acme.localhost:9000", "http://evil.test", false},
		{"acme.localhost:9000", "", false},
	} {
		var ctx fasthttp.RequestCtx
		ctx.Request.SetHost(c.host)
		if c.origin != "" {
			ctx.Request.Header.Set("Origin", c.origin)
		}
		if got := agentOriginAllowed(&ctx); got != c.want {
			t.Errorf("host %s origin %q: %v, want %v", c.host, c.origin, got, c.want)
		}
	}
}
