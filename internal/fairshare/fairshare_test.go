package fairshare

import (
	"sync"
	"testing"
)

func TestLimiter(t *testing.T) {
	l := New(2)
	if !l.Acquire(1) || !l.Acquire(1) {
		t.Fatal("tenant 1 refused under its cap")
	}
	if l.Acquire(1) {
		t.Fatal("tenant 1 went over its cap")
	}
	if !l.Acquire(2) {
		t.Fatal("tenant 2 refused because of tenant 1")
	}
	l.Release(1)
	if !l.Acquire(1) {
		t.Fatal("tenant 1 refused after a release")
	}
	if got := l.Queued(1); got != 2 {
		t.Fatalf("Queued(1) = %d, want 2", got)
	}

	var nilLimiter *Limiter
	if !nilLimiter.Acquire(1) {
		t.Fatal("a nil limiter refused")
	}
	nilLimiter.Release(1)

	unlimited := New(0)
	for range 1000 {
		if !unlimited.Acquire(1) {
			t.Fatal("an uncapped limiter refused")
		}
	}

	// Concurrent use leaves the counts balanced.
	c := New(0)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				c.Acquire(3)
				c.Release(3)
			}
		}()
	}
	wg.Wait()
	if c.Queued(3) != 0 {
		t.Fatalf("unbalanced count %d", c.Queued(3))
	}
}
