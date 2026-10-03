package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	realip "github.com/ferluci/fast-realip"
	"github.com/redis/go-redis/v9"
	"github.com/valyala/fasthttp"
)

// Rule defines a rate limiting rule for a named group of endpoints.
type Rule struct {
	Name              string
	Enabled           bool
	RequestsPerMinute int
}

// Limiter handles rate limiting using Redis.
type Limiter struct {
	redis *redis.Client
	rules map[string]Rule
}

// New creates a new rate limiter.
func New(redisClient *redis.Client) *Limiter {
	return &Limiter{
		redis: redisClient,
		rules: make(map[string]Rule),
	}
}

// AddRule registers a named rate limiting rule.
func (l *Limiter) AddRule(rule Rule) {
	l.rules[rule.Name] = rule
}

// Check checks if the request should be rate limited for the given rule.
func (l *Limiter) Check(ctx *fasthttp.RequestCtx, ruleName string) error {
	rule, ok := l.rules[ruleName]
	if !ok || !rule.Enabled {
		return nil
	}

	clientIP := realip.FromRequest(ctx)
	return l.window(ctx, fmt.Sprintf("rate_limit:%s:%s", ruleName, clientIP), rule.RequestsPerMinute)
}

// CheckTenant counts a request against the tenant's own per-minute budget,
// shared by all its users and API keys. perMinute <= 0 means unlimited. Over
// the limit, it writes a 429 response, counts the hit for the day and returns
// an error.
func (l *Limiter) CheckTenant(ctx *fasthttp.RequestCtx, tenantID, perMinute int) error {
	if perMinute <= 0 || tenantID <= 0 {
		return nil
	}
	err := l.window(ctx, fmt.Sprintf("rate_limit:tenant:%d", tenantID), perMinute)
	if err != nil {
		key := tenantHitsKey(tenantID, time.Now())
		pipe := l.redis.Pipeline()
		pipe.Incr(ctx, key)
		pipe.Expire(ctx, key, 8*24*time.Hour)
		pipe.Exec(ctx)
	}
	return err
}

// TenantHits returns how many of the tenant's requests were refused over its
// limit today.
func (l *Limiter) TenantHits(ctx context.Context, tenantID int) int {
	n, _ := l.redis.Get(ctx, tenantHitsKey(tenantID, time.Now())).Int()
	return n
}

func tenantHitsKey(tenantID int, day time.Time) string {
	return fmt.Sprintf("rate_limit_hits:%d:%s", tenantID, day.UTC().Format("20060102"))
}

// window counts a request in key's sliding one-minute window and refuses it
// over perMinute. It fails open when Redis is unavailable.
func (l *Limiter) window(ctx *fasthttp.RequestCtx, key string, perMinute int) error {

	now := time.Now()
	nowUnix := now.Unix()
	nowNano := now.UnixNano()
	windowStart := strconv.FormatInt(nowUnix-60, 10)

	// Single pipeline: cleanup, add, count, set expiry.
	pipe := l.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", windowStart)
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowUnix), Member: nowNano})
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, time.Minute*2)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil
	}

	count := countCmd.Val()
	limit := int64(perMinute)

	ctx.Response.Header.Set("X-RateLimit-Limit", strconv.Itoa(perMinute))
	ctx.Response.Header.Set("X-RateLimit-Reset", strconv.FormatInt(nowUnix+60, 10))

	if count > limit {
		ctx.Response.Header.Set("X-RateLimit-Remaining", "0")
		ctx.Response.Header.Set("Retry-After", "60")
		ctx.Response.Header.Set("Content-Type", "application/json")
		ctx.SetStatusCode(fasthttp.StatusTooManyRequests)
		ctx.SetBodyString(`{"status":"error","message":"Rate limit exceeded"}`)
		return fmt.Errorf("rate limit exceeded")
	}

	remaining := max(int(limit-count), 0)
	ctx.Response.Header.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	return nil
}
