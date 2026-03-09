package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/allyourbase/ayb/internal/realtime"
)

const (
	queueSize  = 1024
	maxRetries = 3
)

// defaultBackoff holds the production retry delays.
var defaultBackoff = [maxRetries]time.Duration{
	1 * time.Second,
	5 * time.Second,
	25 * time.Second,
}

// Dispatcher receives realtime events and delivers them to matching webhooks.
type Dispatcher struct {
	store     WebhookLister
	deliveryS DeliveryStore // optional — nil disables delivery logging
	client    *http.Client
	logger    *slog.Logger
	queue     chan *realtime.Event
	done      chan struct{}
	wg        sync.WaitGroup
	backoff   [maxRetries]time.Duration // per-instance; tests override without touching globals
}

// NewDispatcher creates a Dispatcher and starts its background worker.
func NewDispatcher(store WebhookLister, logger *slog.Logger) *Dispatcher {
	d := &Dispatcher{
		store:   store,
		client:  &http.Client{Timeout: 10 * time.Second},
		logger:  logger,
		queue:   make(chan *realtime.Event, queueSize),
		done:    make(chan struct{}),
		backoff: defaultBackoff,
	}
	d.wg.Add(1)
	go d.run()
	return d
}

// SetDeliveryStore enables persistent delivery logging.
func (d *Dispatcher) SetDeliveryStore(ds DeliveryStore) {
	d.deliveryS = ds
}

// Enqueue adds an event to the delivery queue.
// Non-blocking: drops events if the queue is full.
func (d *Dispatcher) Enqueue(event *realtime.Event) {
	select {
	case d.queue <- event:
	default:
		d.logger.Warn("webhook queue full, dropping event",
			"table", event.Table, "action", event.Action)
	}
}

// Close signals the worker to stop and waits for it to finish.
func (d *Dispatcher) Close() {
	close(d.done)
	d.wg.Wait()
}

func (d *Dispatcher) run() {
	defer d.wg.Done()
	for {
		select {
		case <-d.done:
			return
		case event, ok := <-d.queue:
			if !ok {
				return
			}
			d.processEvent(event)
		}
	}
}

func (d *Dispatcher) processEvent(event *realtime.Event) {
	hooks, err := d.store.ListEnabled(context.Background())
	if err != nil {
		d.logger.Error("failed to load webhooks", "error", err)
		return
	}

	payload, err := json.Marshal(event)
	if err != nil {
		d.logger.Error("failed to marshal webhook payload", "error", err)
		return
	}

	for i := range hooks {
		if !matches(&hooks[i], event) {
			continue
		}
		d.deliver(&hooks[i], event, payload)
	}
}

func matches(hook *Webhook, event *realtime.Event) bool {
	if len(hook.Tables) > 0 && !contains(hook.Tables, event.Table) {
		return false
	}
	if len(hook.Events) > 0 && !contains(hook.Events, event.Action) {
		return false
	}
	return true
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func (d *Dispatcher) deliver(hook *Webhook, event *realtime.Event, payload []byte) {
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(d.backoff[attempt])
		}

		req, err := http.NewRequest(http.MethodPost, hook.URL, bytes.NewReader(payload))
		if err != nil {
			d.logger.Error("failed to create webhook request", "error", err, "url", hook.URL)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		if hook.Secret != "" {
			req.Header.Set("X-AYB-Signature", Sign(hook.Secret, payload))
		}

		start := time.Now()
		resp, err := d.client.Do(req)
		durationMs := int(time.Since(start).Milliseconds())

		if err != nil {
			d.logger.Warn("webhook delivery failed",
				"url", hook.URL, "attempt", attempt+1, "error", err)
			d.recordDelivery(hook, event, payload, 0, false, attempt+1, durationMs, err.Error(), "")
			continue
		}
		respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			d.recordDelivery(hook, event, payload, resp.StatusCode, true, attempt+1, durationMs, "", string(respBytes))
			return
		}
		d.logger.Warn("webhook returned non-2xx",
			"url", hook.URL, "status", resp.StatusCode, "attempt", attempt+1)
		d.recordDelivery(hook, event, payload, resp.StatusCode, false, attempt+1, durationMs, "", string(respBytes))
	}
	d.logger.Error("webhook delivery exhausted retries", "url", hook.URL, "webhookID", hook.ID)
}

func (d *Dispatcher) recordDelivery(hook *Webhook, event *realtime.Event, payload []byte, statusCode int, success bool, attempt, durationMs int, errMsg, respBody string) {
	if d.deliveryS == nil {
		return
	}
	reqBody := string(payload)
	if len(reqBody) > 4096 {
		reqBody = reqBody[:4096]
	}
	del := &Delivery{
		WebhookID:    hook.ID,
		EventAction:  event.Action,
		EventTable:   event.Table,
		Success:      success,
		StatusCode:   statusCode,
		Attempt:      attempt,
		DurationMs:   durationMs,
		Error:        errMsg,
		RequestBody:  reqBody,
		ResponseBody: respBody,
	}
	if err := d.deliveryS.RecordDelivery(context.Background(), del); err != nil {
		d.logger.Error("failed to record delivery", "error", err)
	}
}

// StartPruner begins periodic cleanup of old delivery logs.
// Does nothing if deliveryS is nil.
func (d *Dispatcher) StartPruner(interval, retention time.Duration) {
	if d.deliveryS == nil {
		return
	}
	d.wg.Add(1)
	go d.runPruner(interval, retention)
}

func (d *Dispatcher) runPruner(interval, retention time.Duration) {
	defer d.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.done:
			return
		case <-ticker.C:
			pruned, err := d.deliveryS.PruneDeliveries(context.Background(), retention)
			if err != nil {
				d.logger.Error("failed to prune webhook deliveries", "error", err)
			} else if pruned > 0 {
				d.logger.Info("pruned old webhook deliveries", "count", pruned)
			}
		}
	}
}

// Sign computes the HMAC-SHA256 signature of body using the given secret.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
