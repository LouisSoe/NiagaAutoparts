// Package cache provides a zero-dependency, in-process key-value cache
// with per-entry TTL and automatic background eviction.
//
// Design notes:
//   - Uses sync.RWMutex so reads (cache hits) don't block each other.
//   - A single background goroutine sweeps expired entries every `cleanupInterval`.
//   - The public API is intentionally identical to the old Redis wrapper so the
//     rest of the codebase requires zero changes.
//   - Stop() must be called on shutdown to release the eviction goroutine.
package cache

import (
	"context"
	"sync"
	"time"
)

// entry is a single cached value with its expiry time.
type entry struct {
	value     string
	expiresAt time.Time
}

// expired returns true if this entry should no longer be served.
func (e entry) expired() bool {
	return time.Now().After(e.expiresAt)
}

// Cache is a thread-safe in-memory key-value store with TTL support.
type Cache struct {
	mu       sync.RWMutex
	items    map[string]entry
	stopCh   chan struct{}
	interval time.Duration
}

// New creates a Cache and starts the background eviction loop.
// cleanupInterval controls how often expired keys are swept; 1–5 minutes is
// a good default for a chatbot workload.
func New(cleanupInterval time.Duration) *Cache {
	c := &Cache{
		items:    make(map[string]entry),
		stopCh:   make(chan struct{}),
		interval: cleanupInterval,
	}
	go c.evictLoop()
	return c
}

// Set stores value under key with the given TTL.
// Calling Set with a zero or negative TTL is a no-op.
func (c *Cache) Set(_ context.Context, key, value string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	c.mu.Lock()
	c.items[key] = entry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

// Get returns the cached value and true, or ("", false) on a miss or expiry.
func (c *Cache) Get(_ context.Context, key string) (string, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || e.expired() {
		return "", false
	}
	return e.value, true
}

// Delete removes a key immediately.
func (c *Cache) Delete(_ context.Context, key string) {
	c.mu.Lock()
	delete(c.items, key)
	c.mu.Unlock()
}

// Len returns the number of entries currently held (including not-yet-evicted
// expired ones). Useful for metrics / health checks.
func (c *Cache) Len() int {
	c.mu.RLock()
	n := len(c.items)
	c.mu.RUnlock()
	return n
}

// Stop shuts down the background eviction goroutine.
// Call this during application shutdown (e.g. in main's defer block).
func (c *Cache) Stop() {
	close(c.stopCh)
}

// ─── background eviction ─────────────────────────────────────────────────────

// evictLoop runs on its own goroutine and periodically removes expired entries.
func (c *Cache) evictLoop() {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.evict()
		case <-c.stopCh:
			return
		}
	}
}

// evict deletes all entries whose TTL has passed.
// It acquires a write lock only long enough to do the deletions.
func (c *Cache) evict() {
	now := time.Now()

	// Collect expired keys with a read lock first (cheaper).
	c.mu.RLock()
	var expired []string
	for k, e := range c.items {
		if now.After(e.expiresAt) {
			expired = append(expired, k)
		}
	}
	c.mu.RUnlock()

	if len(expired) == 0 {
		return
	}

	// Acquire write lock only to delete.
	c.mu.Lock()
	for _, k := range expired {
		// Re-check: another goroutine may have refreshed the entry between
		// the read lock above and this write lock.
		if e, ok := c.items[k]; ok && now.After(e.expiresAt) {
			delete(c.items, k)
		}
	}
	c.mu.Unlock()
}