CREATE TABLE IF NOT EXISTS _ayb_replicas (
	name TEXT PRIMARY KEY,
	host TEXT NOT NULL,
	port INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
	database TEXT NOT NULL,
	ssl_mode TEXT NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('primary', 'replica')),
	state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active', 'draining', 'removed')),
	weight INTEGER NOT NULL DEFAULT 1 CHECK (weight >= 1),
	max_lag_bytes BIGINT NOT NULL DEFAULT 10485760 CHECK (max_lag_bytes >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ayb_replicas_state ON _ayb_replicas (state);

CREATE UNIQUE INDEX IF NOT EXISTS idx_ayb_replicas_single_primary
	ON _ayb_replicas (role)
	WHERE role = 'primary' AND state != 'removed';
