-- Stage 5: Enforce audit immutability at DB permissions/policy level.
-- The application role should only be able to INSERT and SELECT from _ayb_tenant_audit_events.
-- UPDATE and DELETE are revoked to prevent tampering.

-- Create a restrictive policy that only allows INSERT and SELECT.
-- This is enforced via row-level security on the audit events table.

-- First, ensure the table exists (it should from migration 144).
-- Then create a policy that only allows INSERT and SELECT.

-- Revoke UPDATE and DELETE from the application role on audit events.
-- Note: We use a separate role 'ayb_app' for application access.
-- If the application uses the default 'postgres' role, we need to use
-- row-level security or triggers instead.

-- For now, create a trigger-based enforcement that raises an error
-- on any UPDATE or DELETE attempt.

CREATE OR REPLACE FUNCTION _ayb_enforce_audit_immutability()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit events are immutable: UPDATE and DELETE are not allowed';
END;
$$ LANGUAGE plpgsql;

-- Drop trigger if exists, then create
DROP TRIGGER IF EXISTS _ayb_audit_no_update ON _ayb_tenant_audit_events;
DROP TRIGGER IF EXISTS _ayb_audit_no_delete ON _ayb_tenant_audit_events;

CREATE TRIGGER _ayb_audit_no_update
    BEFORE UPDATE ON _ayb_tenant_audit_events
    FOR EACH ROW
    EXECUTE FUNCTION _ayb_enforce_audit_immutability();

CREATE TRIGGER _ayb_audit_no_delete
    BEFORE DELETE ON _ayb_tenant_audit_events
    FOR EACH ROW
    EXECUTE FUNCTION _ayb_enforce_audit_immutability();

-- Add comment documenting the immutability requirement
COMMENT ON TABLE _ayb_tenant_audit_events IS 'Immutable audit log for tenant lifecycle actions. UPDATE and DELETE are blocked by trigger.';
