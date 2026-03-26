package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestNoopBillingService_NoMutation(t *testing.T) {
	t.Parallel()

	svc := NewNoopBillingService()
	ctx := context.Background()
	cust, err := svc.CreateCustomer(ctx, "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", cust.TenantID)

	checkout, err := svc.CreateCheckoutSession(ctx, "tenant-1", PlanPro, "https://ok", "https://cancel")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanFree, checkout.Plan)

	sub, err := svc.GetSubscription(ctx, "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanFree, sub.Plan)
	testutil.Equal(t, PaymentStatusUnpaid, sub.PaymentStatus)

	cancel, err := svc.CancelSubscription(ctx, "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanFree, cancel.Plan)

	err = svc.ReportUsage(ctx, "tenant-1", UsageReport{})
	testutil.NoError(t, err)
}

func TestStripeService_CreateCustomer_CachesCustomerID(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {TenantID: "tenant-1"},
	}}
	adapter := &fakeStripeAdapter{
		createCustomerID: "cus_tenant_1",
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	ctx := context.Background()
	cust, err := svc.CreateCustomer(ctx, "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_tenant_1", cust.StripeCustomerID)
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 1, repo.updateStripeStateCount)
	testutil.Equal(t, 1, adapter.createCustomerCalls)
}

func TestStripeService_CreateCustomer_ExistingCustomerSkipsAdapter(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {TenantID: "tenant-1", StripeCustomerID: "cus_existing"},
	}}
	adapter := &fakeStripeAdapter{}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	cust, err := svc.CreateCustomer(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "cus_existing", cust.StripeCustomerID)
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 0, repo.updateStripeStateCount)
	testutil.Equal(t, 0, adapter.createCustomerCalls)
}

func TestStripeService_CreateCustomer_AdapterError(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {TenantID: "tenant-1"},
	}}
	adapter := &fakeStripeAdapter{
		createCustomerErr: errors.New("stripe down"),
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	_, err := svc.CreateCustomer(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "create stripe customer")
	testutil.ErrorContains(t, err, "stripe down")
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 0, repo.updateStripeStateCount)
	testutil.Equal(t, 1, adapter.createCustomerCalls)
}

func TestStripeService_CreateCustomer_UpdateStripeStateError(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1"},
		},
		updateStripeStateErr: errors.New("persist failed"),
	}
	adapter := &fakeStripeAdapter{
		createCustomerID: "cus_tenant_1",
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	_, err := svc.CreateCustomer(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "persist stripe customer id")
	testutil.ErrorContains(t, err, "persist failed")
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 1, repo.updateStripeStateCount)
	testutil.Equal(t, 1, adapter.createCustomerCalls)
}

func TestStripeService_RecordOrCreate_GetErrorReturned(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{
		getErr: errors.New("db unavailable"),
	}
	svc := &stripeBillingService{repo: repo, cfg: defaultBillingCfg(), adapter: &fakeStripeAdapter{}}

	_, err := svc.recordOrCreate(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "db unavailable")
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 0, repo.createCount)
}

func TestStripeService_RecordOrCreate_CreateConflictFallsBackToGet(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{
		records: map[string]*BillingRecord{
			"tenant-1": {TenantID: "tenant-1", Plan: PlanStarter},
		},
		getErrOnCall: map[int]error{
			1: ErrBillingRecordNotFound,
		},
		createErrOnCall: map[int]error{
			1: ErrBillingConflict,
		},
	}
	svc := &stripeBillingService{repo: repo, cfg: defaultBillingCfg(), adapter: &fakeStripeAdapter{}}

	rec, err := svc.recordOrCreate(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, "tenant-1", rec.TenantID)
	testutil.Equal(t, PlanStarter, rec.Plan)
	testutil.Equal(t, 2, repo.getCount)
	testutil.Equal(t, 1, repo.createCount)
}

func TestStripeService_RecordOrCreate_CreateErrorWrapped(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{
		getErrOnCall: map[int]error{
			1: ErrBillingRecordNotFound,
		},
		createErrOnCall: map[int]error{
			1: errors.New("write failed"),
		},
	}
	svc := &stripeBillingService{repo: repo, cfg: defaultBillingCfg(), adapter: &fakeStripeAdapter{}}

	_, err := svc.recordOrCreate(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "create tenant billing record")
	testutil.ErrorContains(t, err, "write failed")
	testutil.Equal(t, 1, repo.getCount)
	testutil.Equal(t, 1, repo.createCount)
}

func TestStripeService_CreateCheckoutSession_PersistsPlanAndStatus(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {
			TenantID:         "tenant-1",
			StripeCustomerID: "cus_tenant_1",
		},
	}}
	adapter := &fakeStripeAdapter{
		createCheckoutID:    "cs_1",
		createCheckoutURL:   "https://checkout.stripe.com/c/abc",
		createCheckoutSubID: "sub_1",
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())

	session, err := svc.CreateCheckoutSession(context.Background(), "tenant-1", PlanPro, "https://app/ok", "https://app/cancel")
	testutil.NoError(t, err)
	testutil.Equal(t, "cs_1", session.ID)
	testutil.Equal(t, "https://checkout.stripe.com/c/abc", session.URL)
	testutil.Equal(t, PlanPro, session.Plan)
	testutil.Equal(t, 1, repo.upsertCount)
	testutil.Equal(t, PlanPro, repo.records["tenant-1"].Plan)
	testutil.Equal(t, "sub_1", repo.records["tenant-1"].StripeSubscriptionID)
	testutil.Equal(t, PaymentStatusIncomplete, repo.records["tenant-1"].PaymentStatus)
}

func TestStripeService_CreateCheckoutSession_PlanFreeUnsupported(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {
			TenantID:         "tenant-1",
			StripeCustomerID: "cus_tenant_1",
		},
	}}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), &fakeStripeAdapter{}, testNoopLogger())

	_, err := svc.CreateCheckoutSession(context.Background(), "tenant-1", PlanFree, "https://app/ok", "https://app/cancel")
	testutil.ErrorContains(t, err, "unsupported billing plan")
	testutil.Equal(t, 0, repo.upsertCount)
}

func TestStripeService_GetSubscription_MapsPlanAndPersistence(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {
			TenantID:             "tenant-1",
			StripeCustomerID:     "cus_t_1",
			StripeSubscriptionID: "sub_1",
			Plan:                 PlanStarter,
			PaymentStatus:        PaymentStatusUnpaid,
		},
	}}
	adapter := &fakeStripeAdapter{
		getSubscriptionResp: &stripeSubscriptionResponse{
			ID:       "sub_1",
			Status:   "trialing",
			Customer: "cus_t_1",
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
					}{ID: "price_pro"}},
				},
			},
		},
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())
	sub, err := svc.GetSubscription(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanPro, sub.Plan)
	testutil.Equal(t, PaymentStatusTrialing, sub.PaymentStatus)
	testutil.Equal(t, 1, repo.updatePlanPaymentCount)
	testutil.Equal(t, "sub_1", sub.StripeSubscriptionID)
}

func TestStripeService_CancelSubscription_TransitionsToFree(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {
			TenantID:             "tenant-1",
			StripeCustomerID:     "cus_t_1",
			StripeSubscriptionID: "sub_1",
			Plan:                 PlanPro,
			PaymentStatus:        PaymentStatusActive,
		},
	}}
	adapter := &fakeStripeAdapter{
		cancelSubscriptionResp: &stripeSubscriptionResponse{
			ID:       "sub_1",
			Status:   "canceled",
			Customer: "cus_t_1",
		},
	}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())
	sub, err := svc.CancelSubscription(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanFree, sub.Plan)
	testutil.Equal(t, PaymentStatusCanceled, sub.PaymentStatus)
	testutil.Equal(t, 1, repo.updatePlanPaymentCount)
	testutil.Equal(t, PaymentStatusCanceled, repo.records["tenant-1"].PaymentStatus)
	testutil.Equal(t, PlanFree, repo.records["tenant-1"].Plan)
}

func TestStripeService_GetSubscription_RequiresStoredSubscription(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {TenantID: "tenant-1", StripeCustomerID: "cus_t_1"},
	}}
	svc := NewStripeBillingService(repo, defaultBillingCfg(), &fakeStripeAdapter{}, testNoopLogger())
	sub, err := svc.GetSubscription(context.Background(), "tenant-1")
	testutil.NoError(t, err)
	testutil.Equal(t, PlanFree, sub.Plan)
	testutil.Equal(t, PaymentStatusUnpaid, sub.PaymentStatus)
}

func TestStripeService_GetSubscription_UnknownPriceIDReturnsError(t *testing.T) {
	t.Parallel()

	repo := &fakeBillingRepo{records: map[string]*BillingRecord{
		"tenant-1": {
			TenantID:             "tenant-1",
			StripeCustomerID:     "cus_t_1",
			StripeSubscriptionID: "sub_1",
			Plan:                 PlanPro,
			PaymentStatus:        PaymentStatusActive,
		},
	}}
	adapter := &fakeStripeAdapter{
		getSubscriptionResp: &stripeSubscriptionResponse{
			ID:       "sub_1",
			Status:   "active",
			Customer: "cus_t_1",
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
					}{ID: "price_unknown"}},
				},
			},
		},
	}

	svc := NewStripeBillingService(repo, defaultBillingCfg(), adapter, testNoopLogger())
	_, err := svc.GetSubscription(context.Background(), "tenant-1")
	testutil.ErrorContains(t, err, "unknown stripe price id")
	testutil.Equal(t, 0, repo.updatePlanPaymentCount)
}

func TestMapPaymentStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		raw  string
		want PaymentStatus
	}{
		{raw: "active", want: PaymentStatusActive},
		{raw: "past_due", want: PaymentStatusPastDue},
		{raw: "canceled", want: PaymentStatusCanceled},
		{raw: "trialing", want: PaymentStatusTrialing},
		{raw: "incomplete", want: PaymentStatusIncomplete},
		{raw: "unpaid", want: PaymentStatusUnpaid},
		{raw: "unknown_status", want: PaymentStatusIncomplete},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			got := mapPaymentStatus(tc.raw)
			testutil.Equal(t, tc.want, got)
		})
	}
}

func TestStripePriceForPlan(t *testing.T) {
	t.Parallel()

	cfg := defaultBillingCfg()
	testCases := []struct {
		name    string
		plan    Plan
		wantID  string
		wantErr error
	}{
		{name: "starter", plan: PlanStarter, wantID: cfg.StripeStarterPriceID},
		{name: "pro", plan: PlanPro, wantID: cfg.StripeProPriceID},
		{name: "enterprise", plan: PlanEnterprise, wantID: cfg.StripeEnterprisePriceID},
		{name: "free unsupported", plan: PlanFree, wantErr: ErrUnsupportedPlan},
		{name: "unknown unsupported", plan: Plan("legacy"), wantErr: ErrUnsupportedPlan},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := stripePriceForPlan(tc.plan, cfg)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("stripePriceForPlan(%q) error = %v, want %v", tc.plan, err, tc.wantErr)
				}
				testutil.Equal(t, "", got)
				return
			}

			testutil.NoError(t, err)
			testutil.Equal(t, tc.wantID, got)
		})
	}
}

func TestMapPlanFromPriceID(t *testing.T) {
	t.Parallel()

	cfg := defaultBillingCfg()
	testCases := []struct {
		name    string
		priceID string
		want    Plan
		wantErr error
	}{
		{name: "starter", priceID: cfg.StripeStarterPriceID, want: PlanStarter},
		{name: "pro", priceID: cfg.StripeProPriceID, want: PlanPro},
		{name: "enterprise", priceID: cfg.StripeEnterprisePriceID, want: PlanEnterprise},
		{name: "unknown", priceID: "price_unknown", wantErr: ErrUnknownStripePriceID},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := mapPlanFromPriceID(cfg, tc.priceID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("mapPlanFromPriceID(%q) error = %v, want %v", tc.priceID, err, tc.wantErr)
				}
				testutil.Equal(t, Plan(""), got)
				return
			}
			testutil.NoError(t, err)
			testutil.Equal(t, tc.want, got)
		})
	}
}

func TestFirstPriceID(t *testing.T) {
	t.Parallel()

	if got := firstPriceID(nil); got != "" {
		t.Fatalf("firstPriceID(nil) = %q, want empty", got)
	}

	empty := &stripeSubscriptionResponse{}
	if got := firstPriceID(empty); got != "" {
		t.Fatalf("firstPriceID(empty) = %q, want empty", got)
	}

	withPrice := &stripeSubscriptionResponse{}
	withPrice.Items.Data = append(withPrice.Items.Data, struct {
		Price struct {
			ID string `json:"id"`
		} `json:"price"`
	}{
		Price: struct {
			ID string `json:"id"`
		}{ID: "price_pro"},
	})
	if got := firstPriceID(withPrice); got != "price_pro" {
		t.Fatalf("firstPriceID(withPrice) = %q, want %q", got, "price_pro")
	}
}
