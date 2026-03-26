package billing

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/allyourbase/ayb/internal/config"
)

type fakeBillingRepo struct {
	records                map[string]*BillingRecord
	createCount            int
	getCount               int
	upsertCount            int
	updatePlanPaymentCalls []updatePlanPaymentCall
	updatePlanPaymentCount int
	updateStripeStateCalls []updateStripeStateCall
	updateStripeStateCount int

	hasProcessedEventCount  int
	hasProcessedEvents      map[string]bool
	hasProcessedEventErr    error
	recordWebhookEventCount int
	recordWebhookErr        error
	recordedEventIDs        []string

	createErr            error
	createErrOnCall      map[int]error
	getErr               error
	getErrOnCall         map[int]error
	upsertErr            error
	updatePlanPaymentErr error
	updateStripeStateErr error

	usageSync           map[string]int64
	checkpointErr       map[string]error
	upsertCheckpointErr map[string]error
	upsertUsageCalls    []usageCheckpointCall
}

type usageCheckpointCall struct {
	tenantID          string
	usageDate         string
	metric            string
	lastReportedValue int64
}

type updatePlanPaymentCall struct {
	tenantID string
	plan     Plan
	status   PaymentStatus
}

type updateStripeStateCall struct {
	tenantID       string
	customerID     string
	subscriptionID string
}

func (r *fakeBillingRepo) Create(_ context.Context, tenantID string) (*BillingRecord, error) {
	r.createCount++
	if err := r.createErrOnCall[r.createCount]; err != nil {
		return nil, err
	}
	if r.createErr != nil {
		return nil, r.createErr
	}
	rec := &BillingRecord{
		TenantID: tenantID,
		Plan:     PlanFree,
	}
	if r.records == nil {
		r.records = map[string]*BillingRecord{}
	}
	r.records[tenantID] = rec
	return cloneBillingRecord(rec), nil
}

func (r *fakeBillingRepo) Get(_ context.Context, tenantID string) (*BillingRecord, error) {
	r.getCount++
	if err := r.getErrOnCall[r.getCount]; err != nil {
		return nil, err
	}
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.records == nil {
		r.records = map[string]*BillingRecord{}
	}
	rec, ok := r.records[tenantID]
	if !ok {
		return nil, ErrBillingRecordNotFound
	}
	return cloneBillingRecord(rec), nil
}

func (r *fakeBillingRepo) Upsert(_ context.Context, rec *BillingRecord) error {
	r.upsertCount++
	if r.upsertErr != nil {
		return r.upsertErr
	}
	if r.records == nil {
		r.records = map[string]*BillingRecord{}
	}
	r.records[rec.TenantID] = cloneBillingRecord(rec)
	return nil
}

func (r *fakeBillingRepo) UpdatePlanAndPayment(_ context.Context, tenantID string, plan Plan, status PaymentStatus) error {
	r.updatePlanPaymentCount++
	r.updatePlanPaymentCalls = append(r.updatePlanPaymentCalls, updatePlanPaymentCall{
		tenantID: tenantID,
		plan:     plan,
		status:   status,
	})
	if r.updatePlanPaymentErr != nil {
		return r.updatePlanPaymentErr
	}
	rec, ok := r.records[tenantID]
	if !ok {
		return ErrBillingRecordNotFound
	}
	rec.Plan = plan
	rec.PaymentStatus = status
	return nil
}

func (r *fakeBillingRepo) UpdateStripeState(_ context.Context, tenantID, customerID, subscriptionID string) error {
	r.updateStripeStateCount++
	r.updateStripeStateCalls = append(r.updateStripeStateCalls, updateStripeStateCall{
		tenantID:       tenantID,
		customerID:     customerID,
		subscriptionID: subscriptionID,
	})
	if r.updateStripeStateErr != nil {
		return r.updateStripeStateErr
	}
	rec, ok := r.records[tenantID]
	if !ok {
		return ErrBillingRecordNotFound
	}
	if customerID != "" {
		rec.StripeCustomerID = customerID
	}
	if subscriptionID != "" {
		rec.StripeSubscriptionID = subscriptionID
	}
	return nil
}

func (r *fakeBillingRepo) GetBySubscriptionID(_ context.Context, subscriptionID string) (*BillingRecord, error) {
	if r.records == nil {
		r.records = map[string]*BillingRecord{}
	}
	for _, rec := range r.records {
		if rec.StripeSubscriptionID == subscriptionID {
			return cloneBillingRecord(rec), nil
		}
	}
	return nil, ErrBillingRecordNotFound
}

func (r *fakeBillingRepo) HasProcessedEvent(_ context.Context, eventID string) (bool, error) {
	r.hasProcessedEventCount++
	if r.hasProcessedEventErr != nil {
		return false, r.hasProcessedEventErr
	}
	if r.hasProcessedEvents == nil {
		return false, nil
	}
	return r.hasProcessedEvents[eventID], nil
}

func (r *fakeBillingRepo) RecordWebhookEvent(_ context.Context, eventID, eventType string, payload []byte) error {
	r.recordWebhookEventCount++
	r.recordedEventIDs = append(r.recordedEventIDs, eventID)
	if r.recordWebhookErr != nil {
		return r.recordWebhookErr
	}
	if r.hasProcessedEvents == nil {
		r.hasProcessedEvents = map[string]bool{}
	}
	r.hasProcessedEvents[eventID] = true
	return nil
}

func checkpointKey(tenantID, usageDate, metric string) string {
	return tenantID + ":" + usageDate + ":" + metric
}

func (r *fakeBillingRepo) GetUsageSyncCheckpoint(_ context.Context, tenantID string, usageDate string, metric string) (int64, error) {
	key := checkpointKey(tenantID, usageDate, metric)
	if err := r.checkpointErr[key]; err != nil {
		return 0, err
	}
	if r.usageSync == nil {
		return 0, nil
	}
	return r.usageSync[key], nil
}

func (r *fakeBillingRepo) UpsertUsageSyncCheckpoint(_ context.Context, tenantID string, usageDate string, metric string, lastReportedValue int64) error {
	key := checkpointKey(tenantID, usageDate, metric)
	if err := r.upsertCheckpointErr[key]; err != nil {
		return err
	}
	if r.usageSync == nil {
		r.usageSync = map[string]int64{}
	}
	r.usageSync[key] = lastReportedValue
	r.upsertUsageCalls = append(r.upsertUsageCalls, usageCheckpointCall{
		tenantID:          tenantID,
		usageDate:         usageDate,
		metric:            metric,
		lastReportedValue: lastReportedValue,
	})
	return nil
}

func cloneBillingRecord(src *BillingRecord) *BillingRecord {
	if src == nil {
		return nil
	}
	dst := *src
	return &dst
}

type fakeStripeAdapter struct {
	createCustomerErr   error
	createCustomerID    string
	createCustomerCalls int

	createCheckoutID    string
	createCheckoutURL   string
	createCheckoutSubID string
	createCheckoutErr   error

	getSubscriptionResp    *stripeSubscriptionResponse
	getSubscriptionErr     error
	cancelSubscriptionResp *stripeSubscriptionResponse
	cancelSubscriptionErr  error

	sendMeterEventErr map[string]error
	meterCalls        []meterEventCall
}

type meterEventCall struct {
	eventName  string
	customerID string
	value      int64
	identifier string
}

func (a *fakeStripeAdapter) CreateCustomer(_ context.Context, tenantID string) (*stripeCustomerResponse, error) {
	a.createCustomerCalls++
	if a.createCustomerErr != nil {
		return nil, a.createCustomerErr
	}
	if a.createCustomerID == "" {
		a.createCustomerID = "cus_default"
	}
	return &stripeCustomerResponse{ID: a.createCustomerID}, nil
}

func (a *fakeStripeAdapter) CreateCheckoutSession(_ context.Context, tenantID, customerID, priceID, successURL, cancelURL string) (*stripeCheckoutSessionResponse, error) {
	if a.createCheckoutErr != nil {
		return nil, a.createCheckoutErr
	}
	if tenantID == "" || priceID == "" || successURL == "" || cancelURL == "" {
		return nil, errors.New("invalid checkout request")
	}
	if a.createCheckoutID == "" {
		a.createCheckoutID = "cs_default"
	}
	if a.createCheckoutURL == "" {
		a.createCheckoutURL = "https://checkout.example.com"
	}
	return &stripeCheckoutSessionResponse{
		ID:           a.createCheckoutID,
		URL:          a.createCheckoutURL,
		Customer:     "",
		Subscription: a.createCheckoutSubID,
	}, nil
}

func (a *fakeStripeAdapter) GetSubscription(_ context.Context, subscriptionID string) (*stripeSubscriptionResponse, error) {
	if a.getSubscriptionErr != nil {
		return nil, a.getSubscriptionErr
	}
	if a.getSubscriptionResp == nil {
		return &stripeSubscriptionResponse{
			ID:       subscriptionID,
			Status:   "active",
			Customer: "cus_1",
			Items: struct {
				Data []struct {
					Price struct {
						ID string `json:"id"`
					} `json:"price"`
				} `json:"data"`
			}{
				Data: []struct {
					Price struct {
						ID string `json:"id"`
					} `json:"price"`
				}{
					{Price: struct {
						ID string `json:"id"`
					}{ID: "price_starter"}},
				},
			},
		}, nil
	}
	return a.getSubscriptionResp, nil
}

func (a *fakeStripeAdapter) CancelSubscription(_ context.Context, subscriptionID string) (*stripeSubscriptionResponse, error) {
	if a.cancelSubscriptionErr != nil {
		return nil, a.cancelSubscriptionErr
	}
	if a.cancelSubscriptionResp == nil {
		return &stripeSubscriptionResponse{ID: subscriptionID, Status: "canceled", Customer: "cus_1"}, nil
	}
	return a.cancelSubscriptionResp, nil
}

func (a *fakeStripeAdapter) SendMeterEvent(_ context.Context, eventName string, customerID string, value int64, identifier string) error {
	a.meterCalls = append(a.meterCalls, meterEventCall{
		eventName:  eventName,
		customerID: customerID,
		value:      value,
		identifier: identifier,
	})
	if err := a.sendMeterEventErr[eventName]; err != nil {
		return err
	}
	return nil
}

func defaultBillingCfg() config.BillingConfig {
	return config.BillingConfig{
		StripeStarterPriceID:      "price_starter",
		StripeProPriceID:          "price_pro",
		StripeEnterprisePriceID:   "price_enterprise",
		StripeMeterAPIRequests:    "meter.api_requests",
		StripeMeterStorageBytes:   "meter.storage_bytes",
		StripeMeterBandwidthBytes: "meter.bandwidth_bytes",
		StripeMeterFunctionInvs:   "meter.function_invocations",
	}
}

func testNoopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
