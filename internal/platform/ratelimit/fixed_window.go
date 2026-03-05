package ratelimit

import (
	"sync"
	"sync/atomic"
	"time"
)

type FixedWindowLimiter struct {
	max     int64
	window  int64
	ttl     int64
	entries sync.Map // key -> *entry
	calls   atomic.Uint64
}

type entry struct {
	windowStart atomic.Int64
	count       atomic.Int64
	lastSeen    atomic.Int64
}

func NewFixedWindowLimiter(max int, window, ttl time.Duration) *FixedWindowLimiter {
	if max <= 0 {
		max = 30
	}
	if window <= 0 {
		window = time.Second
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &FixedWindowLimiter{max: int64(max), window: int64(window), ttl: int64(ttl)}
}

func (l *FixedWindowLimiter) Allow(key string) bool {
	now := time.Now().UnixNano()
	value, _ := l.entries.LoadOrStore(key, newEntry(now))
	e := value.(*entry)

	e.lastSeen.Store(now)
	for {
		start := e.windowStart.Load()
		if now-start >= l.window {
			if e.windowStart.CompareAndSwap(start, now) {
				e.count.Store(1)
				l.maybeCleanup(now)
				return true
			}
			continue
		}
		current := e.count.Add(1)
		if current <= l.max {
			l.maybeCleanup(now)
			return true
		}
		e.count.Add(-1)
		l.maybeCleanup(now)
		return false
	}
}

func newEntry(now int64) *entry {
	e := &entry{}
	e.windowStart.Store(now)
	e.count.Store(0)
	e.lastSeen.Store(now)
	return e
}

func (l *FixedWindowLimiter) maybeCleanup(now int64) {
	if l.calls.Add(1)%512 != 0 {
		return
	}
	l.entries.Range(func(key, value any) bool {
		e := value.(*entry)
		if now-e.lastSeen.Load() > l.ttl {
			l.entries.Delete(key)
		}
		return true
	})
}
