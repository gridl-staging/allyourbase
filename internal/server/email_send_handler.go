// Package server Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar19_03_go_code_quality_refactoring/allyourbase_dev/internal/server/email_send_handler.go.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/allyourbase/ayb/internal/auth"
	"github.com/allyourbase/ayb/internal/emailtemplates"
	"github.com/allyourbase/ayb/internal/httputil"
	"github.com/allyourbase/ayb/internal/mailer"
)

// emailSendRequest is the JSON body for POST /api/email/send.
type emailSendRequest struct {
	To          json.RawMessage   `json:"to"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	From        string            `json:"from"`
	TemplateKey string            `json:"templateKey"`
	Variables   map[string]string `json:"variables"`
}

// emailSendError reports a per-recipient failure.
type emailSendError struct {
	To    string `json:"to"`
	Error string `json:"error"`
}

// emailSendResponse is the JSON response for POST /api/email/send.
type emailSendResponse struct {
	Sent   int              `json:"sent"`
	Failed int              `json:"failed"`
	Errors []emailSendError `json:"errors,omitempty"`
}

type publicEmailSendInput struct {
	recipients  []string
	hasBody     bool
	hasTemplate bool
}

// handlePublicEmailSend handles POST /api/email/send.
func (s *Server) handlePublicEmailSend(w http.ResponseWriter, r *http.Request) {
	if s.mailer == nil {
		httputil.WriteError(w, http.StatusNotImplemented, "email sending is not configured")
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

	var req emailSendRequest
	if !httputil.DecodeJSON(w, r, &req) {
		return
	}

	maxRecipients := s.cfg.Email.Policy.EffectiveMaxRecipients()
	parsedInput, badRequestMessage := parsePublicEmailSendInput(req)
	if badRequestMessage != "" {
		httputil.WriteError(w, http.StatusBadRequest, badRequestMessage)
		return
	}

	// Enforce from-address policy.
	from, err := s.resolveFromAddress(req.From)
	if err != nil {
		httputil.WriteError(w, http.StatusForbidden, err.Error())
		return
	}

	if len(parsedInput.recipients) > maxRecipients {
		httputil.WriteError(w, http.StatusBadRequest,
			fmt.Sprintf("too many recipients: %d (max %d)", len(parsedInput.recipients), maxRecipients))
		return
	}

	if !s.enforcePublicEmailRateLimit(w, claims) {
		return
	}

	ctx := r.Context()
	subject, html, text, ok := s.resolvePublicEmailContent(w, ctx, req, parsedInput.hasTemplate)
	if !ok {
		return
	}

	// Send to each recipient.
	resp := emailSendResponse{}
	for _, addr := range parsedInput.recipients {
		msg := &mailer.Message{
			To:      addr,
			Subject: subject,
			HTML:    html,
			Text:    text,
			From:    from,
		}
		sendErr := s.mailer.Send(ctx, msg)

		// Audit log (best-effort).
		status := "sent"
		var errMsg string
		if sendErr != nil {
			status = "failed"
			errMsg = sendErr.Error()
		}
		s.logEmailSend(ctx, claims, from, addr, subject, req.TemplateKey, status, errMsg)

		if sendErr != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, emailSendError{To: addr, Error: sendErr.Error()})
		} else {
			resp.Sent++
		}
	}

	httputil.WriteJSON(w, http.StatusOK, resp)
}

// TODO: Document parsePublicEmailSendInput.
func parsePublicEmailSendInput(req emailSendRequest) (publicEmailSendInput, string) {
	parsed := publicEmailSendInput{
		hasBody:     req.HTML != "" || req.Text != "",
		hasTemplate: req.TemplateKey != "",
	}

	recipients, err := parseRecipients(req.To)
	if err != nil {
		return parsed, err.Error()
	}
	if len(recipients) == 0 {
		return parsed, "to is required"
	}
	for _, addr := range recipients {
		if err := httputil.ValidateEmail(addr); err != nil {
			return parsed, fmt.Sprintf("invalid email address: %s", addr)
		}
	}
	parsed.recipients = recipients

	if parsed.hasBody && parsed.hasTemplate {
		return parsed, "cannot specify both html/text and templateKey"
	}
	if !parsed.hasBody && !parsed.hasTemplate {
		return parsed, "must specify either html/text or templateKey"
	}
	if !parsed.hasTemplate && req.Subject == "" {
		return parsed, "subject is required for direct sends"
	}
	return parsed, ""
}

// TODO: Document Server.enforcePublicEmailRateLimit.
func (s *Server) enforcePublicEmailRateLimit(w http.ResponseWriter, claims *auth.Claims) bool {
	if s.emailRL == nil {
		return true
	}

	key := claims.APIKeyID
	if key == "" {
		key = claims.Subject // fall back to user ID
	}

	allowed, _, resetTime := s.emailRL.Allow(key)
	if allowed {
		return true
	}

	retryAfter := int(time.Until(resetTime).Seconds()) + 1
	if retryAfter < 1 {
		retryAfter = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
	httputil.WriteError(w, http.StatusTooManyRequests, "email send rate limit exceeded")
	return false
}

// TODO: Document Server.resolvePublicEmailContent.
func (s *Server) resolvePublicEmailContent(w http.ResponseWriter, ctx context.Context, req emailSendRequest, hasTemplate bool) (string, string, string, bool) {
	if !hasTemplate {
		return req.Subject, req.HTML, req.Text, true
	}

	rendered := s.renderEmailForPublicSend(w, ctx, req.TemplateKey, req.Variables)
	if rendered == nil {
		return "", "", "", false
	}

	subject := rendered.Subject
	if subject == "" && req.Subject != "" {
		subject = req.Subject
	}
	return subject, rendered.HTML, rendered.Text, true
}

// renderEmailForPublicSend renders a template and writes HTTP errors on failure.
// Returns nil when an error has been written to w.
func (s *Server) renderEmailForPublicSend(w http.ResponseWriter, ctx context.Context, key string, vars map[string]string) *emailtemplates.RenderedEmail {
	if s.emailTplSvc == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "email template service not configured")
		return nil
	}
	if vars == nil {
		vars = map[string]string{}
	}
	rendered, err := s.emailTplSvc.Render(ctx, key, vars)
	if err != nil {
		if errors.Is(err, emailtemplates.ErrNoTemplate) {
			httputil.WriteError(w, http.StatusNotFound, "template not found")
		} else if errors.Is(err, emailtemplates.ErrParseFailed) || errors.Is(err, emailtemplates.ErrRenderFailed) {
			httputil.WriteError(w, http.StatusBadRequest, err.Error())
		} else {
			httputil.WriteError(w, http.StatusInternalServerError, "failed to render template")
		}
		return nil
	}
	return rendered
}

// resolveFromAddress validates and resolves the from address against the policy whitelist.
func (s *Server) resolveFromAddress(reqFrom string) (string, error) {
	allowed := s.cfg.Email.EffectiveAllowedFrom()

	if reqFrom == "" {
		if s.cfg.Email.From != "" {
			return s.cfg.Email.From, nil
		}
		return "", fmt.Errorf("no default from address configured")
	}

	for _, a := range allowed {
		if strings.EqualFold(a, reqFrom) {
			return reqFrom, nil
		}
	}
	return "", fmt.Errorf("from address not allowed")
}

// parseRecipients normalizes the "to" field from either a string or []string JSON value.
func parseRecipients(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Try string first.
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		single = strings.TrimSpace(single)
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}

	// Try array.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, fmt.Errorf("to must be a string or array of strings")
	}
	var result []string
	for _, s := range arr {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	return result, nil
}

// logEmailSend writes a row to _ayb_email_send_log. Best-effort: errors are logged but don't fail the request.
func (s *Server) logEmailSend(ctx context.Context, claims *auth.Claims, from, to, subject, templateKey, status, errMsg string) {
	if s.pool == nil {
		return
	}

	var apiKeyID, userID *string
	if claims.APIKeyID != "" {
		apiKeyID = &claims.APIKeyID
	}
	if claims.Subject != "" {
		userID = &claims.Subject
	}

	var templateKeyPtr *string
	if templateKey != "" {
		templateKeyPtr = &templateKey
	}
	var errMsgPtr *string
	if errMsg != "" {
		errMsgPtr = &errMsg
	}

	_, err := s.pool.Exec(ctx,
		`INSERT INTO _ayb_email_send_log (api_key_id, user_id, from_addr, to_addr, subject, template_key, status, error_msg)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		apiKeyID, userID, from, to, subject, templateKeyPtr, status, errMsgPtr,
	)
	if err != nil {
		s.logger.Error("failed to log email send", "error", err, "to", to, "status", status)
	}
}

// SetMailer configures the mailer for the public email send endpoint.
func (s *Server) SetMailer(m mailer.Mailer) {
	s.mailer = m
}

// SetEmailRateLimiter configures the per-key rate limiter for email sends.
func (s *Server) SetEmailRateLimiter(rl *auth.RateLimiter) {
	s.emailRL = rl
}
