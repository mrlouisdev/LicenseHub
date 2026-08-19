package payment

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhookendpoint"
)

const (
	settingWebhookEndpointID = "stripe_webhook_endpoint_id"
	settingWebhookSecret     = "stripe_webhook_secret"
)

var stripeWebhookEvents = []*string{
	stripe.String("checkout.session.completed"),
	stripe.String("invoice.paid"),
	stripe.String("invoice.payment_failed"),
	stripe.String("customer.subscription.deleted"),
	stripe.String("customer.subscription.updated"),
	stripe.String("charge.refunded"),
	stripe.String("charge.dispute.created"),
	stripe.String("charge.dispute.closed"),
	stripe.String("invoice.payment_action_required"),
	stripe.String("customer.subscription.paused"),
	stripe.String("customer.subscription.resumed"),
	stripe.String("customer.subscription.trial_will_end"),
	stripe.String("invoice.upcoming"),
	stripe.String("customer.updated"),
}

// IsLocalhostURL returns true if the URL points to a local address.
// Stripe cannot deliver webhooks to localhost.
func IsLocalhostURL(baseURL string) bool {
	return strings.Contains(baseURL, "localhost") || strings.Contains(baseURL, "127.0.0.1")
}

// SetupWebhookEndpoint ensures a Stripe webhook endpoint is configured.
// It runs synchronously on first attempt, then retries in the background if it fails.
func (h *StripeHandler) SetupWebhookEndpoint(ctx context.Context) {
	if err := h.ensureWebhookEndpoint(ctx); err != nil {
		slog.Error("stripe webhook auto-setup failed, will retry every 60s", "error", err)
		go h.retryWebhookSetup(ctx)
	}
}

func (h *StripeHandler) ensureWebhookEndpoint(ctx context.Context) error {
	webhookURL := strings.TrimRight(h.BaseURL, "/") + "/api/v1/webhook/stripe"

	// Check database for existing endpoint
	endpointID, _ := h.Store.GetSetting(ctx, settingWebhookEndpointID)
	secret, _ := h.Store.GetSetting(ctx, settingWebhookSecret)

	if endpointID != "" && secret != "" {
		// Verify the endpoint still exists in Stripe
		ep, err := webhookendpoint.Get(endpointID, nil)
		if err == nil && !ep.Deleted && ep.Status == "enabled" {
			if ep.URL == webhookURL {
				h.SetWebhookSecret(secret)
				slog.Info("stripe webhook endpoint verified", "endpoint_id", endpointID)
				return nil
			}
			// URL changed (BASE_URL changed) — update the endpoint
			_, err := webhookendpoint.Update(endpointID, &stripe.WebhookEndpointParams{
				URL:           stripe.String(webhookURL),
				EnabledEvents: stripeWebhookEvents,
			})
			if err == nil {
				h.SetWebhookSecret(secret)
				slog.Info("stripe webhook endpoint updated", "endpoint_id", endpointID, "url", webhookURL)
				return nil
			}
			slog.Warn("stripe webhook endpoint update failed, will recreate", "error", err)
		}
		slog.Warn("stripe webhook endpoint not found or disabled, creating new", "old_endpoint_id", endpointID)
	}

	// Create new webhook endpoint
	ep, err := webhookendpoint.New(&stripe.WebhookEndpointParams{
		URL:           stripe.String(webhookURL),
		EnabledEvents: stripeWebhookEvents,
		Description:   stripe.String("Keygate auto-managed webhook"),
		Metadata:      map[string]string{"managed_by": "keygate"},
	})
	if err != nil {
		return fmt.Errorf("create stripe webhook endpoint: %w", err)
	}

	// Stripe only returns Secret at creation time — save both atomically
	if err := h.Store.SetSettings(ctx, map[string]string{
		settingWebhookEndpointID: ep.ID,
		settingWebhookSecret:     ep.Secret,
	}); err != nil {
		return fmt.Errorf("save webhook settings: %w", err)
	}

	h.SetWebhookSecret(ep.Secret)
	slog.Info("stripe webhook endpoint created", "endpoint_id", ep.ID, "url", webhookURL)
	return nil
}

func (h *StripeHandler) retryWebhookSetup(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := h.ensureWebhookEndpoint(ctx); err != nil {
				slog.Error("stripe webhook auto-setup retry failed", "error", err)
				continue
			}
			slog.Info("stripe webhook auto-setup succeeded on retry")
			return
		}
	}
}
