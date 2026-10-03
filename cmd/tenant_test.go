package main

import (
	"testing"

	"github.com/conectdesk/conectdesk/internal/platform"
	"github.com/conectdesk/conectdesk/internal/tenant"
	tmodels "github.com/conectdesk/conectdesk/internal/tenant/models"
	"github.com/conectdesk/conectdesk/internal/testutil"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/logf"
)

func TestTenantHandler(t *testing.T) {
	db := testutil.NewDB(t, "cmd_tenant")
	lo := logf.New(logf.Opts{})
	mgr, err := tenant.New(tenant.Opts{DB: db, Lo: &lo, Enabled: true, BaseDomain: "yourapp.com"})
	if err != nil {
		t.Fatalf("tenant.New: %v", err)
	}
	acme, err := mgr.Create("acme", "Acme", "free")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	gone, err := mgr.Create("gone", "Gone", "free")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := mgr.SetStatus(gone.ID, tmodels.StatusSuspended); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}

	app := &App{tenant: mgr, lo: &lo, i18n: testutil.NewI18n(t)}
	var seen tmodels.Tenant
	h := tenantHandler(app, func(ctx *fasthttp.RequestCtx) {
		seen, _ = ctx.UserValue(tenantCtxKey).(tmodels.Tenant)
	})

	for _, c := range []struct {
		host, path string
		code, id   int
	}{
		{"acme.yourapp.com", "/api/v1/config", fasthttp.StatusOK, acme.ID},
		{"gone.yourapp.com", "/api/v1/config", fasthttp.StatusForbidden, 0},
		{"nobody.yourapp.com", "/api/v1/config", fasthttp.StatusNotFound, 0},
		{"yourapp.com", "/api/v1/config", fasthttp.StatusNotFound, 0},
		{"yourapp.com", "/health", fasthttp.StatusOK, 0},
	} {
		seen = tmodels.Tenant{}
		var ctx fasthttp.RequestCtx
		ctx.Request.SetRequestURI(c.path)
		ctx.Request.Header.SetHost(c.host)
		h(&ctx)
		if ctx.Response.StatusCode() != c.code || seen.ID != c.id {
			t.Errorf("%s%s: status %d tenant %d; want %d tenant %d",
				c.host, c.path, ctx.Response.StatusCode(), seen.ID, c.code, c.id)
		}
	}
}

// Behind a proxy that ends TLS, redirects keep the https scheme.
func TestTenantHandlerForwardedProto(t *testing.T) {
	db := testutil.NewDB(t, "cmd_tenant_proto")
	lo := logf.New(logf.Opts{})
	mgr, err := tenant.New(tenant.Opts{DB: db, Lo: &lo, Enabled: true, BaseDomain: "yourapp.com"})
	if err != nil {
		t.Fatalf("tenant.New: %v", err)
	}
	app := &App{tenant: mgr, lo: &lo, i18n: testutil.NewI18n(t), platform: &platform.Manager{}, platformHost: "admin.yourapp.com"}
	h := tenantHandler(app, func(ctx *fasthttp.RequestCtx) {})

	for proto, want := range map[string]string{
		"https": "https://admin.yourapp.com/platform",
		"":      "http://admin.yourapp.com/platform",
	} {
		var ctx fasthttp.RequestCtx
		ctx.Request.SetRequestURI("/")
		ctx.Request.Header.SetHost("admin.yourapp.com")
		if proto != "" {
			ctx.Request.Header.Set("X-Forwarded-Proto", proto)
		}
		h(&ctx)
		if got := string(ctx.Response.Header.Peek("Location")); got != want {
			t.Errorf("X-Forwarded-Proto %q: Location %q, want %q", proto, got, want)
		}
	}
}
