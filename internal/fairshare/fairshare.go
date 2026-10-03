// Package fairshare caps how much of a shared queue one tenant can take, so a
// tenant flooding it has its own work dropped instead of everyone's.
package fairshare

import "sync"

// Limiter counts each tenant's items in a queue.
type Limiter struct {
	max int

	mu sync.Mutex
	n  map[int]int
}

// New returns a Limiter allowing max queued items per tenant; max <= 0 means
// no cap.
func New(max int) *Limiter {
	return &Limiter{max: max, n: make(map[int]int)}
}

// Acquire takes a slot for one of the tenant's items, reporting false when the
// tenant already has max items queued. Each true must be paired with Release.
func (l *Limiter) Acquire(tenantID int) bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.max > 0 && l.n[tenantID] >= l.max {
		return false
	}
	l.n[tenantID]++
	return true
}

// Release frees a slot Acquire took.
func (l *Limiter) Release(tenantID int) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.n[tenantID] <= 1 {
		delete(l.n, tenantID)
		return
	}
	l.n[tenantID]--
}

// Queued returns how many of the tenant's items are queued.
func (l *Limiter) Queued(tenantID int) int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.n[tenantID]
}
