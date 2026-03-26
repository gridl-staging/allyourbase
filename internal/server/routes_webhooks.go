package server

import (
	"strings"

	"github.com/allyourbase/ayb/internal/billing"
	"github.com/go-chi/chi/v5"
)

// registerWebhookRoutes mounts webhook ingress routes: SMS status callback
// (Twilio, form-encoded), support email webhook (SendGrid, multipart), and
// Stripe billing webhook.
func (s *Server) registerWebhookRoutes(r chi.Router) {
	// SMS delivery webhook (Twilio sends form-encoded, not JSON).
	r.Post("/webhooks/sms/status", s.handleSMSDeliveryWebhook)

	// Support inbound email webhook (SendGrid multipart/form-data).
	if s.cfg.Support.Enabled {
		r.Post("/webhooks/support/email", s.handleSupportEmailWebhook)
	}

	// Stripe webhook endpoint for billing lifecycle events.
	if s.cfg != nil && s.cfg.Billing.Provider == "stripe" {
		secret := strings.TrimSpace(s.cfg.Billing.StripeWebhookSecret)
		if secret != "" && s.pool != nil {
			handler := billing.NewWebhookHandler(
				billing.NewStore(s.pool),
				s.cfg.Billing,
				secret,
				s.currentLogger(),
			)
			r.Post("/webhooks/stripe", handler.HandleWebhook)
		}
	}
}
