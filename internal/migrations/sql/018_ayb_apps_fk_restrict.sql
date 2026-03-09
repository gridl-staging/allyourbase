-- Fix: change app_id FK from ON DELETE SET NULL to ON DELETE RESTRICT.
-- ON DELETE SET NULL silently converts app-scoped keys into unrestricted
-- user-scoped keys when an app is deleted concurrently with key creation.
-- ON DELETE RESTRICT forces the application to explicitly detach keys
-- before deleting an app, preventing accidental privilege escalation.
ALTER TABLE _ayb_api_keys DROP CONSTRAINT IF EXISTS _ayb_api_keys_app_id_fkey;
ALTER TABLE _ayb_api_keys
    ADD CONSTRAINT _ayb_api_keys_app_id_fkey
    FOREIGN KEY (app_id) REFERENCES _ayb_apps(id) ON DELETE RESTRICT;
