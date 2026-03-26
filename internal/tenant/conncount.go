package tenant

import "sync"

// TenantConnCounter tracks live connections per tenant in memory.
type TenantConnCounter struct {
	mu      sync.Mutex
	tenants map[string]int64
}

// NewTenantConnCounter creates a new tenant connection counter.
func NewTenantConnCounter() *TenantConnCounter {
	return &TenantConnCounter{
		tenants: make(map[string]int64),
	}
}

// Admit checks whether a new connection is allowed and increments the live
// connection count if it is. The check and increment happen atomically.
func (c *TenantConnCounter) Admit(tenantID string, hardLimit, softLimit *int) (allowed bool, softWarning bool, count int64) {
	if tenantID == "" || c == nil {
		return true, false, 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	current := c.tenants[tenantID]
	hard, hasHard := normalizeIntLimit(hardLimit)
	soft, hasSoft := normalizeIntLimit(softLimit)

	if hasHard && current >= hard {
		return false, false, current
	}

	next := current + 1
	c.tenants[tenantID] = next

	if hasSoft && next >= soft && (!hasHard || next < hard) {
		softWarning = true
	}

	return true, softWarning, next
}

// Release decrements the live connection count for a tenant.
// Counts are clamped to zero to avoid negative values.
func (c *TenantConnCounter) Release(tenantID string) {
	if tenantID == "" || c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	count := c.tenants[tenantID] - 1
	if count <= 0 {
		delete(c.tenants, tenantID)
		return
	}
	c.tenants[tenantID] = count
}
