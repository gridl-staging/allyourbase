package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeRouteLister struct {
	domains []DomainBinding
	err     error
}

func (f *fakeRouteLister) ListDomainsForRouting(_ context.Context) ([]DomainBinding, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.domains, nil
}

type fakeTombstoneReaper struct {
	count  int64
	err    error
	called int
}

func (f *fakeTombstoneReaper) ReapExpiredTombstones(_ context.Context) (int64, error) {
	f.called++
	if f.err != nil {
		return 0, f.err
	}
	return f.count, nil
}

func TestHostRouteMiddleware_ActiveDomain(t *testing.T) {
	entries := []RouteEntry{
		{DomainID: "dom-1", Hostname: "app.example.com", Environment: "production", Status: StatusActive},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		re, ok := CustomDomainRouteFromContext(r.Context())
		if !ok {
			t.Fatal("expected route entry in context")
		}
		if re.Hostname != "app.example.com" {
			t.Errorf("expected hostname app.example.com, got %s", re.Hostname)
		}
		if re.Environment != "production" {
			t.Errorf("expected environment production, got %s", re.Environment)
		}
		if re.Status != StatusActive {
			t.Errorf("expected status active, got %s", re.Status)
		}
		if re.DomainID != "dom-1" {
			t.Errorf("expected DomainID dom-1, got %s", re.DomainID)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "app.example.com"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("expected next handler to be called")
	}
}

func TestHostRouteMiddleware_TombstonedDomain(t *testing.T) {
	entries := []RouteEntry{
		{DomainID: "dom-old", Hostname: "old.example.com", Environment: "production", Status: StatusTombstoned},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	// Assert DomainID is preserved in route table even for tombstoned entries.
	entry, ok := s.lookupRoute("old.example.com")
	if !ok {
		t.Fatal("expected tombstoned entry in route table")
	}
	if entry.DomainID != "dom-old" {
		t.Errorf("expected DomainID dom-old in tombstoned route entry, got %s", entry.DomainID)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "old.example.com"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if nextCalled {
		t.Error("expected next handler NOT to be called for tombstoned domain")
	}

	if rec.Code != http.StatusMisdirectedRequest {
		t.Errorf("expected status 421, got %d", rec.Code)
	}

	var errResp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if errResp.Code != 421 {
		t.Errorf("expected code 421, got %d", errResp.Code)
	}
	if errResp.Message != "hostname is no longer active" {
		t.Errorf("expected message 'hostname is no longer active', got %s", errResp.Message)
	}
}

func TestHostRouteMiddleware_LapsedDomainRoutedDuringGrace(t *testing.T) {
	entries := []RouteEntry{
		{DomainID: "dom-lapsed", Hostname: "lapsed.example.com", Environment: "production", Status: StatusVerificationLapsed},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		re, ok := CustomDomainRouteFromContext(r.Context())
		if !ok {
			t.Fatal("expected route entry in context for lapsed domain during grace period")
		}
		if re.Status != StatusVerificationLapsed {
			t.Errorf("expected status verification_lapsed, got %s", re.Status)
		}
		if re.DomainID != "dom-lapsed" {
			t.Errorf("expected DomainID dom-lapsed, got %s", re.DomainID)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "lapsed.example.com"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("expected next handler to be called for lapsed domain during grace period")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestNewRouteTable_TombstonedRemainsWinnerWhenSeenFirst(t *testing.T) {
	rt := NewRouteTable([]RouteEntry{
		{DomainID: "dom-tomb", Hostname: "dupe.example.com", Environment: "production", Status: StatusTombstoned},
		{DomainID: "dom-active", Hostname: "DUPE.example.com", Environment: "production", Status: StatusActive},
	})

	entry, ok := rt.Lookup("dupe.example.com")
	if !ok {
		t.Fatal("expected route entry for duplicate hostname")
	}
	if entry.Status != StatusTombstoned {
		t.Fatalf("expected tombstoned entry to remain winner, got %s", entry.Status)
	}
	if entry.DomainID != "dom-tomb" {
		t.Fatalf("expected tombstoned winner to keep DomainID dom-tomb, got %s", entry.DomainID)
	}
}

func TestHostRouteMiddleware_UnknownHost(t *testing.T) {
	entries := []RouteEntry{
		{Hostname: "app.example.com", Environment: "production", Status: StatusActive},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		re, ok := CustomDomainRouteFromContext(r.Context())
		if ok {
			t.Error("expected no route entry in context for unknown host")
		}
		_ = re
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "unknown.example.com"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("expected next handler to be called for unknown host")
	}
}

func TestHostRouteMiddleware_HostWithPort(t *testing.T) {
	entries := []RouteEntry{
		{Hostname: "app.example.com", Environment: "production", Status: StatusActive},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		re, ok := CustomDomainRouteFromContext(r.Context())
		if !ok {
			t.Fatal("expected route entry in context")
		}
		if re.Hostname != "app.example.com" {
			t.Errorf("expected hostname app.example.com, got %s", re.Hostname)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "app.example.com:8443"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("expected next handler to be called")
	}
}

func TestHostRouteMiddleware_BareLocalhost(t *testing.T) {
	entries := []RouteEntry{
		{Hostname: "localhost", Environment: "development", Status: StatusActive},
	}
	rt := NewRouteTable(entries)

	s := &Server{}
	s.setRouteTable(rt)

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		re, ok := CustomDomainRouteFromContext(r.Context())
		if !ok {
			t.Fatal("expected route entry in context")
		}
		if re.Hostname != "localhost" {
			t.Errorf("expected hostname localhost, got %s", re.Hostname)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()

	s.hostRouteMiddleware(next).ServeHTTP(rec, req)

	if !nextCalled {
		t.Error("expected next handler to be called")
	}
}

func TestLoadRouteTable(t *testing.T) {
	now := time.Now()
	redirect := "www"
	lister := &fakeRouteLister{
		domains: []DomainBinding{
			{
				ID:           "1",
				Hostname:     "active.example.com",
				Environment:  "production",
				Status:       StatusActive,
				RedirectMode: &redirect,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:           "2",
				Hostname:     "tombstoned.example.com",
				Environment:  "production",
				Status:       StatusTombstoned,
				TombstonedAt: &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
			{
				ID:          "3",
				Hostname:    "pending.example.com",
				Environment: "production",
				Status:      StatusPendingVerification,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "4",
				Hostname:    "verified.example.com",
				Environment: "production",
				Status:      StatusVerified,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:          "5",
				Hostname:    "lapsed.example.com",
				Environment: "production",
				Status:      StatusVerificationLapsed,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}

	rt, err := loadRouteTable(context.Background(), lister, nil)
	if err != nil {
		t.Fatalf("loadRouteTable failed: %v", err)
	}

	if len(rt) != 3 {
		t.Errorf("expected 3 entries (active + tombstoned + lapsed), got %d", len(rt))
	}

	entry, ok := rt["active.example.com"]
	if !ok {
		t.Error("expected active.example.com in route table")
	}
	if entry.Status != StatusActive {
		t.Errorf("expected status active, got %s", entry.Status)
	}
	if entry.Environment != "production" {
		t.Errorf("expected environment production, got %s", entry.Environment)
	}
	if entry.RedirectMode == nil || *entry.RedirectMode != "www" {
		t.Error("expected redirect mode to be set")
	}
	if entry.DomainID != "1" {
		t.Errorf("expected DomainID 1, got %s", entry.DomainID)
	}

	entry, ok = rt["tombstoned.example.com"]
	if !ok {
		t.Error("expected tombstoned.example.com in route table")
	}
	if entry.Status != StatusTombstoned {
		t.Errorf("expected status tombstoned, got %s", entry.Status)
	}
	if entry.DomainID != "2" {
		t.Errorf("expected DomainID 2, got %s", entry.DomainID)
	}

	entry, ok = rt["lapsed.example.com"]
	if !ok {
		t.Error("expected lapsed.example.com in route table (grace period routing)")
	}
	if entry.Status != StatusVerificationLapsed {
		t.Errorf("expected status verification_lapsed, got %s", entry.Status)
	}
	if entry.DomainID != "5" {
		t.Errorf("expected DomainID 5, got %s", entry.DomainID)
	}

	if _, ok := rt["pending.example.com"]; ok {
		t.Error("pending.example.com should not be in route table")
	}
	if _, ok := rt["verified.example.com"]; ok {
		t.Error("verified.example.com should not be in route table")
	}
}

func TestLoadRouteTable_Error(t *testing.T) {
	lister := &fakeRouteLister{
		err: context.DeadlineExceeded,
	}

	rt, err := loadRouteTable(context.Background(), lister, nil)
	if err == nil {
		t.Error("expected error, got nil")
	}
	if rt != nil {
		t.Error("expected nil route table on error")
	}
}

func TestLoadRouteTable_TombstonedWinsForDuplicateHostname(t *testing.T) {
	now := time.Now()
	lister := &fakeRouteLister{
		domains: []DomainBinding{
			{
				ID:          "1",
				Hostname:    "App.Example.com",
				Environment: "production",
				Status:      StatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			{
				ID:           "2",
				Hostname:     "app.example.com",
				Environment:  "production",
				Status:       StatusTombstoned,
				TombstonedAt: &now,
				CreatedAt:    now,
				UpdatedAt:    now,
			},
		},
	}

	rt, err := loadRouteTable(context.Background(), lister, nil)
	if err != nil {
		t.Fatalf("loadRouteTable failed: %v", err)
	}

	if len(rt) != 1 {
		t.Fatalf("expected 1 entry for duplicate hostname, got %d", len(rt))
	}

	entry, ok := rt.Lookup("APP.EXAMPLE.COM")
	if !ok {
		t.Fatal("expected normalized hostname lookup to succeed")
	}
	if entry.Status != StatusTombstoned {
		t.Fatalf("expected tombstoned entry to win for duplicate hostname, got %s", entry.Status)
	}
	if entry.DomainID != "2" {
		t.Fatalf("expected tombstoned winner to keep DomainID 2, got %s", entry.DomainID)
	}
}

func TestDomainTombstoneReapHandler_Success(t *testing.T) {
	reaper := &fakeTombstoneReaper{
		count: 5,
	}

	handler := DomainTombstoneReapHandler(reaper, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Errorf("handler failed: %v", err)
	}

	if reaper.called != 1 {
		t.Errorf("expected reaper called 1 time, got %d", reaper.called)
	}
}

func TestDomainTombstoneReapHandler_Error(t *testing.T) {
	reaper := &fakeTombstoneReaper{
		err: context.DeadlineExceeded,
	}

	handler := DomainTombstoneReapHandler(reaper, nil)
	err := handler(context.Background(), nil)
	if err == nil {
		t.Error("expected error, got nil")
	}

	if reaper.called != 1 {
		t.Errorf("expected reaper called 1 time, got %d", reaper.called)
	}
}

func TestDomainRouteSyncHandler_Success(t *testing.T) {
	now := time.Now()
	lister := &fakeRouteLister{
		domains: []DomainBinding{
			{
				ID:          "1",
				Hostname:    "sync.example.com",
				Environment: "production",
				Status:      StatusActive,
				CreatedAt:   now,
				UpdatedAt:   now,
			},
		},
	}

	s := &Server{}

	handler := DomainRouteSyncHandler(s, lister, nil)
	err := handler(context.Background(), nil)
	if err != nil {
		t.Errorf("handler failed: %v", err)
	}

	entry, ok := s.lookupRoute("sync.example.com")
	if !ok {
		t.Error("expected sync.example.com in route table after sync")
	}
	if entry.Hostname != "sync.example.com" {
		t.Errorf("expected hostname sync.example.com, got %s", entry.Hostname)
	}
	if entry.DomainID != "1" {
		t.Errorf("expected DomainID 1, got %s", entry.DomainID)
	}
}

func TestDomainRouteSyncHandler_Error(t *testing.T) {
	lister := &fakeRouteLister{
		err: context.DeadlineExceeded,
	}

	s := &Server{}
	s.setRouteTable(NewRouteTable([]RouteEntry{
		{DomainID: "dom-existing", Hostname: "existing.example.com", Environment: "production", Status: StatusActive},
	}))

	handler := DomainRouteSyncHandler(s, lister, nil)
	err := handler(context.Background(), nil)
	if err == nil {
		t.Error("expected error, got nil")
	}

	entry, ok := s.lookupRoute("existing.example.com")
	if !ok {
		t.Error("expected existing.example.com still in route table after failed sync")
	}
	if entry.DomainID != "dom-existing" {
		t.Errorf("expected DomainID dom-existing preserved after failed sync, got %s", entry.DomainID)
	}
}

func TestRouteTableLookup(t *testing.T) {
	entries := []RouteEntry{
		{Hostname: "example.com", Environment: "prod", Status: StatusActive},
		{Hostname: "test.example.com", Environment: "test", Status: StatusTombstoned},
	}
	rt := NewRouteTable(entries)

	entry, ok := rt.Lookup("example.com")
	if !ok {
		t.Error("expected to find example.com")
	}
	if entry.Environment != "prod" {
		t.Errorf("expected environment prod, got %s", entry.Environment)
	}

	entry, ok = rt.Lookup("test.example.com")
	if !ok {
		t.Error("expected to find test.example.com")
	}
	if entry.Status != StatusTombstoned {
		t.Errorf("expected status tombstoned, got %s", entry.Status)
	}

	_, ok = rt.Lookup("unknown.example.com")
	if ok {
		t.Error("expected not to find unknown.example.com")
	}
}

func TestRouteTableLookup_NormalizesHostname(t *testing.T) {
	entries := []RouteEntry{
		{Hostname: "App.Example.com", Environment: "prod", Status: StatusActive},
	}
	rt := NewRouteTable(entries)

	entry, ok := rt.Lookup("app.example.com")
	if !ok {
		t.Fatal("expected lowercase lookup to find mixed-case entry")
	}
	if entry.Hostname != "app.example.com" {
		t.Fatalf("expected normalized stored hostname app.example.com, got %s", entry.Hostname)
	}

	entry, ok = rt.Lookup("APP.EXAMPLE.COM")
	if !ok {
		t.Fatal("expected uppercase lookup to be normalized")
	}
	if entry.Status != StatusActive {
		t.Fatalf("expected status active, got %s", entry.Status)
	}
}

func TestCustomDomainRouteFromContext(t *testing.T) {
	entry := RouteEntry{
		DomainID:    "dom-ctx",
		Hostname:    "ctx.example.com",
		Environment: "staging",
		Status:      StatusActive,
	}

	ctx := context.WithValue(context.Background(), customDomainRouteKey{}, entry)
	re, ok := CustomDomainRouteFromContext(ctx)
	if !ok {
		t.Error("expected to find route entry in context")
	}
	if re.Hostname != "ctx.example.com" {
		t.Errorf("expected hostname ctx.example.com, got %s", re.Hostname)
	}
	if re.DomainID != "dom-ctx" {
		t.Errorf("expected DomainID dom-ctx, got %s", re.DomainID)
	}

	_, ok = CustomDomainRouteFromContext(context.Background())
	if ok {
		t.Error("expected not to find route entry in empty context")
	}
}
