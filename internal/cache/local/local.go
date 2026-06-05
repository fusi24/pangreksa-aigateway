// Package local provides an L1 in-process entitlement cache backed by sync.Map.
// Entries are evicted lazily: an entry is considered expired when its ExpiresAt
// time has passed; the expired value is deleted on the next read attempt.
//
// This cache is the first tier in the three-layer entitlement resolution strategy:
//
//	L1 (this package)  ~50 ns latency
//	L2 (redis package) ~0.5 ms latency
//	L3 (HTTP)          ~20 ms latency
package local

import (
	"sync"
	"time"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

// entry wraps a UserEntitlement with its storage timestamp.
type entry struct {
	// ent is the cached entitlement value.
	ent *model.UserEntitlement
}

// Cache is a thread-safe in-process entitlement cache backed by sync.Map.
// Entries are evicted after their ExpiresAt time has passed (lazy eviction on Get).
//
// Thread-safety: safe for concurrent use.
type Cache struct {
	// m is the underlying sync.Map; keys are userID strings, values are *entry.
	m sync.Map
}

// NewCache constructs an empty local Cache.
func NewCache() *Cache {
	return &Cache{}
}

// Get returns the cached UserEntitlement for userID. The second return value is
// false when no entry exists or the entry's ExpiresAt has passed (in which case
// the stale entry is deleted).
func (c *Cache) Get(userID string) (*model.UserEntitlement, bool) {
	raw, ok := c.m.Load(userID)
	if !ok {
		return nil, false
	}

	e, ok := raw.(*entry)
	if !ok {
		// defensive: unexpected type stored; evict and miss.
		c.m.Delete(userID)
		return nil, false
	}

	// lazy expiry: return miss and remove the stale entry.
	if time.Now().After(e.ent.ExpiresAt) {
		c.m.Delete(userID)
		return nil, false
	}

	return e.ent, true
}

// Set stores or replaces the entitlement for ent.UserID.
// Callers must ensure ent is non-nil and that ent.ExpiresAt is set to the
// desired eviction time.
func (c *Cache) Set(ent *model.UserEntitlement) {
	c.m.Store(ent.UserID, &entry{ent: ent})
}

// Delete removes the entitlement for userID from the cache. It is a no-op if
// the entry does not exist.
func (c *Cache) Delete(userID string) {
	c.m.Delete(userID)
}
