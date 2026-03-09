package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
)

// ListAllMessages implements messageStore for fakeMsgStore — returns all messages
// across all users, sorted by created_at DESC, with total count for pagination.
func (f *fakeMsgStore) ListAllMessages(_ context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	total := len(f.messages)

	// Sort by created_at DESC (newest first) — reverse a copy.
	sorted := make([]smsMessage, len(f.messages))
	copy(sorted, f.messages)
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	if offset >= len(sorted) {
		return []adminSMSMessage{}, total, nil
	}
	sorted = sorted[offset:]
	if limit > 0 && limit < len(sorted) {
		sorted = sorted[:limit]
	}

	result := make([]adminSMSMessage, len(sorted))
	for i, m := range sorted {
		result[i] = adminSMSMessage{
			ID:                m.ID,
			UserID:            m.UserID,
			ToPhone:           m.ToPhone,
			Body:              m.Body,
			Provider:          m.Provider,
			ProviderMessageID: m.ProviderMessageID,
			Status:            m.Status,
			ErrorMessage:      m.ErrorMessage,
			CreatedAt:         m.CreatedAt,
			UpdatedAt:         m.UpdatedAt,
		}
	}
	return result, total, nil
}

// --- Admin SMS Messages tests ---

type adminSMSListResponse struct {
	Items      []adminSMSMessage `json:"items"`
	Page       int               `json:"page"`
	PerPage    int               `json:"perPage"`
	TotalItems int               `json:"totalItems"`
	TotalPages int               `json:"totalPages"`
}

func TestAdminSMSMessages_NoPool_Returns404(t *testing.T) {
	t.Parallel()
	// Server with no msgStore → handler returns 404.
	srv := newMessagingTestServer(t, func(s *Server) {
		s.msgStore = nil
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/messages", nil)
	srv.handleAdminSMSMessages(w, req)

	testutil.Equal(t, http.StatusNotFound, w.Code)
}

func TestAdminSMSMessages_EmptyResult(t *testing.T) {
	t.Parallel()
	srv := newMessagingTestServer(t) // fakeMsgStore starts empty

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/messages", nil)
	srv.handleAdminSMSMessages(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp adminSMSListResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 0, len(resp.Items))
	testutil.Equal(t, 1, resp.Page)
	testutil.Equal(t, 50, resp.PerPage)
	testutil.Equal(t, 0, resp.TotalItems)
	testutil.Equal(t, 0, resp.TotalPages)
}

func TestAdminSMSMessages_ReturnsList(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	store.InsertMessage(ctx, "user-1", "+12025551234", "first", "twilio")
	time.Sleep(time.Millisecond) // ensure different timestamps
	store.InsertMessage(ctx, "user-2", "+12025555678", "second", "twilio")

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/messages", nil)
	srv.handleAdminSMSMessages(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp adminSMSListResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 2, len(resp.Items))
	testutil.Equal(t, 2, resp.TotalItems)

	// Sorted by created_at DESC — newest first.
	testutil.Equal(t, "second", resp.Items[0].Body)
	testutil.Equal(t, "user-2", resp.Items[0].UserID)
	testutil.Equal(t, "+12025555678", resp.Items[0].ToPhone)

	testutil.Equal(t, "first", resp.Items[1].Body)
	testutil.Equal(t, "user-1", resp.Items[1].UserID)
	testutil.Equal(t, "+12025551234", resp.Items[1].ToPhone)
}

func TestAdminSMSMessages_Pagination(t *testing.T) {
	t.Parallel()
	store := &fakeMsgStore{}
	srv := newMessagingTestServer(t, func(s *Server) { s.msgStore = store })

	ctx := context.Background()
	for i := 0; i < 15; i++ {
		store.InsertMessage(ctx, "user-1", "+12025551234", fmt.Sprintf("msg-%02d", i), "twilio")
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/sms/messages?page=2&perPage=10", nil)
	srv.handleAdminSMSMessages(w, req)

	testutil.Equal(t, http.StatusOK, w.Code)

	var resp adminSMSListResponse
	testutil.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	testutil.Equal(t, 5, len(resp.Items))   // 15 total, page 2 of 10 = items 11-15
	testutil.Equal(t, 2, resp.Page)
	testutil.Equal(t, 10, resp.PerPage)
	testutil.Equal(t, 15, resp.TotalItems)
	testutil.Equal(t, 2, resp.TotalPages)
}
