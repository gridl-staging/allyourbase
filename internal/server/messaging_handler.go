package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/sms"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSMSBodyLength = 1600

// adminSMSMessage is smsMessage with UserID exposed for admin endpoints.
type adminSMSMessage struct {
	ID                string    `json:"id"`
	UserID            string    `json:"user_id"`
	ToPhone           string    `json:"to"`
	Body              string    `json:"body"`
	Provider          string    `json:"provider"`
	ProviderMessageID string    `json:"message_id"`
	Status            string    `json:"status"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// smsMessage represents a row in the _ayb_sms_messages table.
type smsMessage struct {
	ID                string    `json:"id"`
	UserID            string    `json:"-"`
	APIKeyID          *string   `json:"-"`
	ToPhone           string    `json:"to"`
	Body              string    `json:"body"`
	Provider          string    `json:"provider"`
	ProviderMessageID string    `json:"message_id"`
	Status            string    `json:"status"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// messageStore abstracts SMS message persistence for testability.
type messageStore interface {
	InsertMessage(ctx context.Context, userID, toPhone, body, provider string) (string, error)
	UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error
	UpdateMessageFailed(ctx context.Context, id, errMsg string) error
	GetMessage(ctx context.Context, id, userID string) (*smsMessage, error)
	ListMessages(ctx context.Context, userID string, limit, offset int) ([]smsMessage, error)
	ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error)
	UpdateDeliveryStatus(ctx context.Context, providerMsgID, status, errMsg string) error
}

// pgMessageStore implements messageStore using a PostgreSQL connection pool.
type pgMessageStore struct {
	pool *pgxpool.Pool
}

func (s *pgMessageStore) InsertMessage(ctx context.Context, userID, toPhone, body, provider string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO _ayb_sms_messages (user_id, to_phone, body, provider, status)
		 VALUES ($1, $2, $3, $4, 'pending')
		 RETURNING id`,
		userID, toPhone, body, provider,
	).Scan(&id)
	return id, err
}

func (s *pgMessageStore) UpdateMessageSent(ctx context.Context, id, providerMsgID, status string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_sms_messages
		 SET provider_message_id = $1, status = $2, updated_at = now()
		 WHERE id = $3`,
		providerMsgID, status, id,
	)
	return err
}

func (s *pgMessageStore) UpdateMessageFailed(ctx context.Context, id, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_sms_messages
		 SET status = 'failed', error_message = $1, updated_at = now()
		 WHERE id = $2`,
		errMsg, id,
	)
	return err
}

func (s *pgMessageStore) GetMessage(ctx context.Context, id, userID string) (*smsMessage, error) {
	var m smsMessage
	err := s.pool.QueryRow(ctx,
		`SELECT id, to_phone, body, provider, provider_message_id, status, error_message, created_at, updated_at
		 FROM _ayb_sms_messages WHERE id = $1 AND user_id = $2`,
		id, userID,
	).Scan(&m.ID, &m.ToPhone, &m.Body, &m.Provider, &m.ProviderMessageID, &m.Status, &m.ErrorMessage, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (s *pgMessageStore) ListAllMessages(ctx context.Context, limit, offset int) ([]adminSMSMessage, int, error) {
	var total int
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM _ayb_sms_messages`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT id, user_id, to_phone, body, provider, provider_message_id, status, error_message, created_at, updated_at
		 FROM _ayb_sms_messages
		 ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var msgs []adminSMSMessage
	for rows.Next() {
		var m adminSMSMessage
		if err := rows.Scan(&m.ID, &m.UserID, &m.ToPhone, &m.Body, &m.Provider, &m.ProviderMessageID, &m.Status, &m.ErrorMessage, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, 0, err
		}
		msgs = append(msgs, m)
	}
	return msgs, total, rows.Err()
}

func (s *pgMessageStore) ListMessages(ctx context.Context, userID string, limit, offset int) ([]smsMessage, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, to_phone, body, provider, provider_message_id, status, error_message, created_at, updated_at
		 FROM _ayb_sms_messages WHERE user_id = $1
		 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []smsMessage
	for rows.Next() {
		var m smsMessage
		if err := rows.Scan(&m.ID, &m.ToPhone, &m.Body, &m.Provider, &m.ProviderMessageID, &m.Status, &m.ErrorMessage, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

// deliveryStatusRank returns a numeric rank for a Twilio message status so that
// UpdateDeliveryStatus can refuse to regress a message to an earlier lifecycle position.
// Terminal statuses (delivered, failed, etc.) share the highest rank and can overwrite each other.
func deliveryStatusRank(status string) int {
	switch status {
	case "pending":
		return 0
	case "accepted":
		return 1
	case "queued":
		return 2
	case "sending":
		return 3
	case "sent":
		return 4
	default: // delivered, undelivered, failed, read, canceled
		return 5
	}
}

func (s *pgMessageStore) UpdateDeliveryStatus(ctx context.Context, providerMsgID, status, errMsg string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE _ayb_sms_messages
		 SET status = $1, error_message = $2, updated_at = now()
		 WHERE provider_message_id = $3
		 AND (
		     CASE status
		         WHEN 'pending'  THEN 0
		         WHEN 'accepted' THEN 1
		         WHEN 'queued'   THEN 2
		         WHEN 'sending'  THEN 3
		         WHEN 'sent'     THEN 4
		         ELSE                 5
		     END
		     <=
		     CASE $1::text
		         WHEN 'pending'  THEN 0
		         WHEN 'accepted' THEN 1
		         WHEN 'queued'   THEN 2
		         WHEN 'sending'  THEN 3
		         WHEN 'sent'     THEN 4
		         ELSE                 5
		     END
		 )`,
		status, errMsg, providerMsgID,
	)
	return err
}

// smsSendInput holds validated and normalized fields from an SMS send request.
type smsSendInput struct {
	Phone string // E.164 normalized
	Body  string
}

// validateSMSSendBody decodes the JSON request body and validates phone + body.
// Returns (input, httpStatus, errorMessage). Status is 0 on success.
func (s *Server) validateSMSSendBody(r *http.Request) (*smsSendInput, int, string) {
	var body struct {
		To   string `json:"to"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, http.StatusBadRequest, "invalid request body"
	}
	if body.To == "" {
		return nil, http.StatusBadRequest, "to is required"
	}
	phone, err := sms.NormalizePhone(body.To)
	if err != nil {
		return nil, http.StatusBadRequest, "invalid phone number"
	}
	if !sms.IsAllowedCountry(phone, s.smsAllowedCountries) {
		return nil, http.StatusBadRequest, "phone number country not allowed"
	}
	if body.Body == "" {
		return nil, http.StatusBadRequest, "body is required"
	}
	if len(body.Body) > maxSMSBodyLength {
		return nil, http.StatusBadRequest, "body exceeds maximum length"
	}
	return &smsSendInput{Phone: phone, Body: body.Body}, 0, ""
}

// handleMessagingSMSSend handles POST /api/messaging/sms/send.
func (s *Server) handleMessagingSMSSend(w http.ResponseWriter, r *http.Request) {
	if s.smsProvider == nil {
		httputil.WriteErrorWithDocURL(w, http.StatusNotFound,
			"SMS is not enabled",
			"https://allyourbase.io/guide/messaging#sms")
		return
	}

	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	if err := auth.CheckWriteScope(claims); err != nil {
		httputil.WriteError(w, http.StatusForbidden, "api key scope does not permit write operations")
		return
	}

	input, status, errMsg := s.validateSMSSendBody(r)
	if status != 0 {
		httputil.WriteError(w, status, errMsg)
		return
	}

	ctx := r.Context()
	msgID, err := s.msgStore.InsertMessage(ctx, claims.Subject, input.Phone, input.Body, s.smsProviderName)
	if err != nil {
		s.logger.Error("failed to insert SMS message", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to create message")
		return
	}

	result, err := s.smsProvider.Send(ctx, input.Phone, input.Body)
	if err != nil {
		_ = s.msgStore.UpdateMessageFailed(ctx, msgID, err.Error())
		httputil.WriteError(w, http.StatusInternalServerError, "failed to send SMS")
		return
	}

	sendStatus := result.Status
	if sendStatus == "" {
		sendStatus = "queued"
	}
	_ = s.msgStore.UpdateMessageSent(ctx, msgID, result.MessageID, sendStatus)

	httputil.WriteJSON(w, http.StatusOK, map[string]string{
		"id":         msgID,
		"message_id": result.MessageID,
		"status":     sendStatus,
		"to":         input.Phone,
	})
}

// handleMessagingSMSList handles GET /api/messaging/sms/messages.
func (s *Server) handleMessagingSMSList(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 100 {
		limit = 100
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	msgs, err := s.msgStore.ListMessages(r.Context(), claims.Subject, limit, offset)
	if err != nil {
		s.logger.Error("failed to list SMS messages", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	if msgs == nil {
		msgs = []smsMessage{}
	}

	httputil.WriteJSON(w, http.StatusOK, msgs)
}

// handleMessagingSMSGet handles GET /api/messaging/sms/messages/{id}.
func (s *Server) handleMessagingSMSGet(w http.ResponseWriter, r *http.Request) {
	claims := auth.ClaimsFromContext(r.Context())
	if claims == nil {
		httputil.WriteError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	id := chi.URLParam(r, "id")
	if id == "" {
		httputil.WriteError(w, http.StatusBadRequest, "message id is required")
		return
	}

	msg, err := s.msgStore.GetMessage(r.Context(), id, claims.Subject)
	if err != nil {
		s.logger.Error("failed to get SMS message", "error", err)
		httputil.WriteError(w, http.StatusInternalServerError, "failed to get message")
		return
	}
	if msg == nil {
		httputil.WriteError(w, http.StatusNotFound, "message not found")
		return
	}

	httputil.WriteJSON(w, http.StatusOK, msg)
}

// handleSMSDeliveryWebhook handles POST /api/webhooks/sms/status.
// Twilio sends application/x-www-form-urlencoded status callbacks.
// TODO: Twilio request signature verification — requires webhook URL in config + auth token on server
func (s *Server) handleSMSDeliveryWebhook(w http.ResponseWriter, r *http.Request) {
	messageSid := r.FormValue("MessageSid")
	if messageSid == "" {
		httputil.WriteError(w, http.StatusBadRequest, "MessageSid is required")
		return
	}

	messageStatus := r.FormValue("MessageStatus")
	if messageStatus == "" {
		// Twilio always sends MessageStatus. An empty value is malformed — skip
		// the update to avoid corrupting the stored status, but return 200 so
		// Twilio does not retry (Twilio retries on any non-2xx response).
		return
	}

	errorCode := r.FormValue("ErrorCode")
	errorMessage := r.FormValue("ErrorMessage")

	var errMsg string
	if errorCode != "" {
		errMsg = fmt.Sprintf("error %s: %s", errorCode, errorMessage)
	}

	if err := s.msgStore.UpdateDeliveryStatus(r.Context(), messageSid, messageStatus, errMsg); err != nil {
		s.logger.Error("failed to update SMS delivery status", "error", err, "message_sid", messageSid)
	}

	httputil.WriteJSON(w, http.StatusOK, map[string]string{})
}
