package auth

import (
	"sync"
	"time"
)

// TokenDenyList tracks revoked session IDs for the remaining access-token TTL.
type TokenDenyList struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

// NewTokenDenyList creates an empty in-memory deny list.
func NewTokenDenyList() *TokenDenyList {
	return &TokenDenyList{
		entries: make(map[string]time.Time),
	}
}

// Add marks a session as denied until now+ttl.
func (d *TokenDenyList) Add(sessionID string, ttl time.Duration) {
	if sessionID == "" {
		return
	}
	expiresAt := time.Now().Add(ttl)

	d.mu.Lock()
	d.entries[sessionID] = expiresAt
	d.mu.Unlock()
}

// IsDenied returns true when a session is still within its deny window.
// Expired entries are lazily evicted.
func (d *TokenDenyList) IsDenied(sessionID string) bool {
	if sessionID == "" {
		return false
	}

	d.mu.RLock()
	expiresAt, ok := d.entries[sessionID]
	d.mu.RUnlock()
	if !ok {
		return false
	}

	if time.Now().Before(expiresAt) {
		return true
	}

	d.mu.Lock()
	// Remove only if the map still points to the same expired timestamp.
	if current, found := d.entries[sessionID]; found && current.Equal(expiresAt) {
		delete(d.entries, sessionID)
	}
	d.mu.Unlock()

	return false
}

// Len returns the number of non-expired entries.
func (d *TokenDenyList) Len() int {
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()

	for sessionID, expiresAt := range d.entries {
		if !now.Before(expiresAt) {
			delete(d.entries, sessionID)
		}
	}
	return len(d.entries)
}
