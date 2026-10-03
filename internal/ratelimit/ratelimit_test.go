package ratelimit

import (
	"context"
	"os"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

// newTestLimiter connects to the Redis at CONECTDESK_TEST_REDIS, or the test
// Redis on 127.0.0.1:6380, skipping when there is none.
func newTestLimiter(t *testing.T) (*Limiter, *redis.Client) {
	t.Helper()
	addr := os.Getenv("CONECTDESK_TEST_REDIS")
	if addr == "" {
		addr = "127.0.0.1:6380"
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("no test Redis at %s: %v", addr, err)
	}
	rdb.FlushDB(context.Background())
	t.Cleanup(func() { rdb.FlushDB(context.Background()); rdb.Close() })
	return New(rdb), rdb
}

// TestCheckTenant checks each tenant has its own per-minute budget, shared by
// all its callers, and that refusals are counted.
func TestCheckTenant(t *testing.T) {
	l, _ := newTestLimiter(t)
	req := func(tenantID, limit int) (bool, int) {
		var ctx fasthttp.RequestCtx
		ctx.Init(&fasthttp.Request{}, nil, nil)
		err := l.CheckTenant(&ctx, tenantID, limit)
		return err == nil, ctx.Response.StatusCode()
	}

	for i := range 5 {
		if ok, _ := req(2, 5); !ok {
			t.Fatalf("acme request %d refused under its limit", i+1)
		}
	}
	if ok, code := req(2, 5); ok || code != fasthttp.StatusTooManyRequests {
		t.Fatalf("acme's 6th request: allowed=%v code=%d, want 429", ok, code)
	}
	if ok, _ := req(3, 5); !ok {
		t.Fatal("globex refused because of acme's traffic")
	}
	if ok, _ := req(2, 0); !ok {
		t.Fatal("an unlimited plan was refused")
	}
	req(2, 5)
	if n := l.TenantHits(context.Background(), 2); n != 2 {
		t.Fatalf("acme hits = %d, want 2", n)
	}
	if n := l.TenantHits(context.Background(), 3); n != 0 {
		t.Fatalf("globex hits = %d, want 0", n)
	}
}
