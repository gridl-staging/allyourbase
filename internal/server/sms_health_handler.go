package server

import (
	"net/http"
	"strconv"

	"github.com/allyourbase/ayb/internal/httputil"
)

// smsWindowStats holds aggregated SMS stats for a time window.
type smsWindowStats struct {
	Sent           int     `json:"sent"`
	Confirmed      int     `json:"confirmed"`
	Failed         int     `json:"failed"`
	ConversionRate float64 `json:"conversion_rate"`
}

// handleAdminSMSHealth returns SMS delivery stats for today, last 7 days, and last 30 days.
func (s *Server) handleAdminSMSHealth(w http.ResponseWriter, r *http.Request) {
	if s.pool == nil {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()

	query := `
		SELECT
			COALESCE(SUM(count) FILTER (WHERE date = CURRENT_DATE), 0),
			COALESCE(SUM(confirm_count) FILTER (WHERE date = CURRENT_DATE), 0),
			COALESCE(SUM(fail_count) FILTER (WHERE date = CURRENT_DATE), 0),
			COALESCE(SUM(count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '6 days'), 0),
			COALESCE(SUM(confirm_count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '6 days'), 0),
			COALESCE(SUM(fail_count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '6 days'), 0),
			COALESCE(SUM(count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '29 days'), 0),
			COALESCE(SUM(confirm_count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '29 days'), 0),
			COALESCE(SUM(fail_count) FILTER (WHERE date >= CURRENT_DATE - INTERVAL '29 days'), 0)
		FROM _ayb_sms_daily_counts
		WHERE date >= CURRENT_DATE - INTERVAL '29 days'`

	var todaySent, todayConfirmed, todayFailed int
	var weekSent, weekConfirmed, weekFailed int
	var monthSent, monthConfirmed, monthFailed int

	err := s.pool.QueryRow(ctx, query).Scan(
		&todaySent, &todayConfirmed, &todayFailed,
		&weekSent, &weekConfirmed, &weekFailed,
		&monthSent, &monthConfirmed, &monthFailed,
	)
	if err != nil {
		s.logger.Error("SMS health query error", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to query SMS stats")
		return
	}

	today := smsWindowStats{Sent: todaySent, Confirmed: todayConfirmed, Failed: todayFailed, ConversionRate: conversionRate(todaySent, todayConfirmed)}
	week := smsWindowStats{Sent: weekSent, Confirmed: weekConfirmed, Failed: weekFailed, ConversionRate: conversionRate(weekSent, weekConfirmed)}
	month := smsWindowStats{Sent: monthSent, Confirmed: monthConfirmed, Failed: monthFailed, ConversionRate: conversionRate(monthSent, monthConfirmed)}

	resp := map[string]any{
		"today":   today,
		"last_7d": week,
		"last_30d": month,
	}

	// Warn when today's conversion rate is below 10% with meaningful volume.
	if todaySent > 0 && today.ConversionRate < 10 {
		resp["warning"] = "low conversion rate"
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// handleAdminSMSMessages returns a paginated list of all SMS messages for admin.
func (s *Server) handleAdminSMSMessages(w http.ResponseWriter, r *http.Request) {
	if s.msgStore == nil {
		http.NotFound(w, r)
		return
	}

	page := 1
	perPage := 50
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("perPage"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			perPage = n
		}
	}
	if perPage > 200 {
		perPage = 200
	}

	offset := (page - 1) * perPage
	msgs, total, err := s.msgStore.ListAllMessages(r.Context(), perPage, offset)
	if err != nil {
		s.logger.Error("failed to list admin SMS messages", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	if msgs == nil {
		msgs = []adminSMSMessage{}
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + perPage - 1) / perPage
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"items":      msgs,
		"page":       page,
		"perPage":    perPage,
		"totalItems": total,
		"totalPages": totalPages,
	})
}

// handleAdminSMSSend handles POST /api/admin/sms/send.
// Validates and sends an SMS via the configured provider. No message is stored
// in the database — admin sends are for testing delivery, not production tracking.
func (s *Server) handleAdminSMSSend(w http.ResponseWriter, r *http.Request) {
	if s.smsProvider == nil {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound,
			"SMS is not enabled",
			"https://allyourbase.io/guide/messaging#sms")
		return
	}

	input, status, errMsg := s.validateSMSSendBody(r)
	if status != 0 {
		httputil.WriteError(w, status, errMsg)
		return
	}

	result, err := s.smsProvider.Send(r.Context(), input.Phone, input.Body)
	if err != nil {
		s.logger.Error("admin SMS send failed", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendStatus := result.Status
	if sendStatus == "" {
		sendStatus = "queued"
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"message_id": result.MessageID,
		"status":     sendStatus,
		"to":         input.Phone,
	})
}

// conversionRate calculates confirmed/sent * 100, returning 0 when sent is 0.
func conversionRate(sent, confirmed int) float64 {
	if sent == 0 {
		return 0
	}
	return float64(confirmed) / float64(sent) * 100
}
