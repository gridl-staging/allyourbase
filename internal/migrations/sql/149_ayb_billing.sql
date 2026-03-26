-- Stage 2: Tenant billing lifecycle metadata for Stripe/plan state.

CREATE TABLE IF NOT EXISTS _ayb_billing (
    tenant_id            UUID        NOT NULL PRIMARY KEY REFERENCES _ayb_tenants(id) ON DELETE CASCADE,
    stripe_customer_id   TEXT        NULL,
    stripe_subscription_id TEXT      NULL,
    plan                 TEXT        NOT NULL DEFAULT 'free',
    payment_status       TEXT        NOT NULL DEFAULT 'unpaid',
    trial_start_at        TIMESTAMPTZ NULL,
    trial_end_at          TIMESTAMPTZ NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_ayb_billing_plan
        CHECK (plan IN ('free', 'starter', 'pro', 'enterprise')),
    CONSTRAINT chk_ayb_billing_payment_status
        CHECK (payment_status IN ('unpaid', 'active', 'past_due', 'canceled', 'trialing', 'incomplete'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_billing_stripe_customer_id
    ON _ayb_billing (stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_billing_stripe_subscription_id
    ON _ayb_billing (stripe_subscription_id)
    WHERE stripe_subscription_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ayb_billing_plan
    ON _ayb_billing (plan);

CREATE INDEX IF NOT EXISTS idx_ayb_billing_payment_status
    ON _ayb_billing (payment_status);
