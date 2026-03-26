// Package vault Store manages encrypted secret persistence in Postgres with methods for creating, reading, updating, and deleting secrets.
package vault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSecretNotFound = errors.New("vault secret not found")
var ErrSecretAlreadyExists = errors.New("vault secret already exists")

// SecretMetadata represents non-sensitive secret metadata.
type SecretMetadata struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists encrypted secrets in Postgres.
type Store struct {
	pool  *pgxpool.Pool
	vault *Vault
}

func NewStore(pool *pgxpool.Pool, vault *Vault) *Store {
	return &Store{pool: pool, vault: vault}
}

// SetSecret creates or updates a secret with the given name and value, encrypting the value and upserting it to the database. If a secret with the name already exists, its value is overwritten.
func (s *Store) SetSecret(ctx context.Context, name string, value []byte) error {
	normalizedName, err := NormalizeSecretName(name)
	if err != nil {
		return err
	}
	if s == nil || s.pool == nil || s.vault == nil {
		return errors.New("vault store is not initialized")
	}

	ciphertext, nonce, err := s.vault.Encrypt(value)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO _ayb_vault_secrets (name, encrypted_value, nonce)
		VALUES ($1, $2, $3)
		ON CONFLICT (name) DO UPDATE SET
			encrypted_value = EXCLUDED.encrypted_value,
			nonce = EXCLUDED.nonce,
			updated_at = NOW()
	`, normalizedName, ciphertext, nonce)
	if err != nil {
		return fmt.Errorf("upserting secret %q: %w", normalizedName, err)
	}
	return nil
}

// CreateSecret inserts a new secret with the given name and value, returning ErrSecretAlreadyExists if a secret with that name already exists.
func (s *Store) CreateSecret(ctx context.Context, name string, value []byte) error {
	normalizedName, err := NormalizeSecretName(name)
	if err != nil {
		return err
	}
	if s == nil || s.pool == nil || s.vault == nil {
		return errors.New("vault store is not initialized")
	}

	ciphertext, nonce, err := s.vault.Encrypt(value)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
		INSERT INTO _ayb_vault_secrets (name, encrypted_value, nonce)
		VALUES ($1, $2, $3)
	`, normalizedName, ciphertext, nonce)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return fmt.Errorf("%w: %s", ErrSecretAlreadyExists, normalizedName)
		}
		return fmt.Errorf("creating secret %q: %w", normalizedName, err)
	}
	return nil
}

// GetSecret retrieves a secret by name, decrypts it, and returns the plaintext value, returning ErrSecretNotFound if no such secret exists.
func (s *Store) GetSecret(ctx context.Context, name string) ([]byte, error) {
	normalizedName, err := NormalizeSecretName(name)
	if err != nil {
		return nil, err
	}
	if s == nil || s.pool == nil || s.vault == nil {
		return nil, errors.New("vault store is not initialized")
	}

	var ciphertext []byte
	var nonce []byte
	err = s.pool.QueryRow(ctx,
		`SELECT encrypted_value, nonce FROM _ayb_vault_secrets WHERE name = $1`,
		normalizedName,
	).Scan(&ciphertext, &nonce)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, normalizedName)
		}
		return nil, fmt.Errorf("querying secret %q: %w", normalizedName, err)
	}

	plaintext, err := s.vault.Decrypt(ciphertext, nonce)
	if err != nil {
		return nil, fmt.Errorf("decrypting secret %q: %w", normalizedName, err)
	}
	return plaintext, nil
}

// UpdateSecret encrypts and updates an existing secret with a new value, returning ErrSecretNotFound if no secret with that name exists.
func (s *Store) UpdateSecret(ctx context.Context, name string, value []byte) error {
	normalizedName, err := NormalizeSecretName(name)
	if err != nil {
		return err
	}
	if s == nil || s.pool == nil || s.vault == nil {
		return errors.New("vault store is not initialized")
	}

	ciphertext, nonce, err := s.vault.Encrypt(value)
	if err != nil {
		return err
	}

	tag, err := s.pool.Exec(ctx, `
		UPDATE _ayb_vault_secrets
		SET encrypted_value = $2, nonce = $3, updated_at = NOW()
		WHERE name = $1
	`, normalizedName, ciphertext, nonce)
	if err != nil {
		return fmt.Errorf("updating secret %q: %w", normalizedName, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, normalizedName)
	}
	return nil
}

// ListSecrets returns metadata for all secrets in the store without decrypting their values, ordered by name.
func (s *Store) ListSecrets(ctx context.Context) ([]SecretMetadata, error) {
	if s == nil || s.pool == nil {
		return nil, errors.New("vault store is not initialized")
	}

	rows, err := s.pool.Query(ctx,
		`SELECT name, created_at, updated_at FROM _ayb_vault_secrets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing secrets: %w", err)
	}
	defer rows.Close()

	var secrets []SecretMetadata
	for rows.Next() {
		var secret SecretMetadata
		if err := rows.Scan(&secret.Name, &secret.CreatedAt, &secret.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning secret metadata: %w", err)
		}
		secrets = append(secrets, secret)
	}
	if secrets == nil {
		secrets = []SecretMetadata{}
	}
	return secrets, rows.Err()
}

// GetAllSecretsDecrypted returns all vault secrets as decrypted key-value pairs.
// Satisfies the edgefunc.VaultSecretProvider interface.
func (s *Store) GetAllSecretsDecrypted(ctx context.Context) (map[string]string, error) {
	if s == nil || s.pool == nil || s.vault == nil {
		return nil, errors.New("vault store is not initialized")
	}

	rows, err := s.pool.Query(ctx,
		`SELECT name, encrypted_value, nonce FROM _ayb_vault_secrets ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("querying all secrets: %w", err)
	}
	defer rows.Close()

	secrets := make(map[string]string)
	for rows.Next() {
		var name string
		var ciphertext, nonce []byte
		if err := rows.Scan(&name, &ciphertext, &nonce); err != nil {
			return nil, fmt.Errorf("scanning secret row: %w", err)
		}
		plaintext, err := s.vault.Decrypt(ciphertext, nonce)
		if err != nil {
			return nil, fmt.Errorf("decrypting secret %q: %w", name, err)
		}
		secrets[name] = string(plaintext)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating secrets: %w", err)
	}
	return secrets, nil
}

// DeleteSecret removes a secret from the store by name, returning ErrSecretNotFound if no such secret exists.
func (s *Store) DeleteSecret(ctx context.Context, name string) error {
	normalizedName, err := NormalizeSecretName(name)
	if err != nil {
		return err
	}
	if s == nil || s.pool == nil {
		return errors.New("vault store is not initialized")
	}

	tag, err := s.pool.Exec(ctx, `DELETE FROM _ayb_vault_secrets WHERE name = $1`, normalizedName)
	if err != nil {
		return fmt.Errorf("deleting secret %q: %w", normalizedName, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", ErrSecretNotFound, normalizedName)
	}
	return nil
}
