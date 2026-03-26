package tenant

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const rlsAuthenticatedRole = "ayb_authenticated"

// SchemaProvisioner manages tenant schema lifecycle operations.
type SchemaProvisioner struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewSchemaProvisioner creates a schema provisioner.
func NewSchemaProvisioner(pool *pgxpool.Pool, logger *slog.Logger) *SchemaProvisioner {
	return &SchemaProvisioner{
		pool:   pool,
		logger: logger,
	}
}

// ProvisionSchema creates a tenant schema and grants usage for authenticated RLS role.
func (p *SchemaProvisioner) ProvisionSchema(ctx context.Context, slug string) error {
	if p == nil || p.pool == nil || slug == "" {
		return nil
	}

	schemaName := pgx.Identifier{slug}.Sanitize()
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, schemaName)); err != nil {
		return fmt.Errorf("creating schema %q: %w", slug, err)
	}
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`GRANT USAGE ON SCHEMA %s TO %s`, schemaName, rlsAuthenticatedRole)); err != nil {
		return fmt.Errorf("granting schema usage for %q: %w", slug, err)
	}

	if p.logger != nil {
		p.logger.Info("tenant schema provisioned", "slug", slug)
	}
	return nil
}

// DropSchema drops a tenant schema with CASCADE.
func (p *SchemaProvisioner) DropSchema(ctx context.Context, slug string) error {
	if p == nil || p.pool == nil || slug == "" {
		return nil
	}

	schemaName := pgx.Identifier{slug}.Sanitize()
	if _, err := p.pool.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS %s CASCADE`, schemaName)); err != nil {
		return fmt.Errorf("dropping schema %q: %w", slug, err)
	}

	if p.logger != nil {
		p.logger.Info("tenant schema dropped", "slug", slug)
	}
	return nil
}
