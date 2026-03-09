-- Add scope and allowed_tables columns for granular API key permissions.
-- scope: '*' (full access), 'readonly' (GET only), 'readwrite' (CRUD, no admin)
-- allowed_tables: empty array = all tables, or restrict to specific tables
ALTER TABLE _ayb_api_keys ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT '*';
ALTER TABLE _ayb_api_keys ADD COLUMN IF NOT EXISTS allowed_tables TEXT[] NOT NULL DEFAULT '{}';
