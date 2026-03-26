//go:build integration

package vault_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/allyourbase/ayb/internal/migrations"
	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/allyourbase/ayb/internal/vault"
)

var sharedPG *testutil.PGContainer

func TestMain(m *testing.M) {
	ctx := context.Background()
	pg, cleanup := testutil.StartPostgresForTestMain(ctx)
	sharedPG = pg
	code := m.Run()
	cleanup()
	os.Exit(code)
}

func resetAndMigrate(t *testing.T, ctx context.Context) {
	t.Helper()
	_, err := sharedPG.Pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public")
	if err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	runner := migrations.NewRunner(sharedPG.Pool, testutil.DiscardLogger())
	testutil.NoError(t, runner.Bootstrap(ctx))
	_, err = runner.Run(ctx)
	testutil.NoError(t, err)
}

func testStore(t *testing.T) *vault.Store {
	t.Helper()
	v, err := vault.New(bytes.Repeat([]byte{0x42}, 32))
	testutil.NoError(t, err)
	return vault.NewStore(sharedPG.Pool, v)
}

func TestStoreSetGetListDeleteSecretLifecycle(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	testutil.NoError(t, store.SetSecret(ctx, "API_KEY", []byte("super-secret-value")))

	got, err := store.GetSecret(ctx, "API_KEY")
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal([]byte("super-secret-value"), got), "fetched value should match")

	list, err := store.ListSecrets(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(list))
	testutil.Equal(t, "API_KEY", list[0].Name)
	testutil.True(t, !list[0].CreatedAt.IsZero(), "CreatedAt must be populated")
	testutil.True(t, !list[0].UpdatedAt.IsZero(), "UpdatedAt must be populated")

	testutil.NoError(t, store.DeleteSecret(ctx, "API_KEY"))
	_, err = store.GetSecret(ctx, "API_KEY")
	testutil.True(t, errors.Is(err, vault.ErrSecretNotFound), "deleted secret should return not found")
}

func TestStoreSetSecretUpsertUpdatesExisting(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	testutil.NoError(t, store.SetSecret(ctx, "DUPLICATE", []byte("value-1")))
	testutil.NoError(t, store.SetSecret(ctx, "DUPLICATE", []byte("value-2")))

	got, err := store.GetSecret(ctx, "DUPLICATE")
	testutil.NoError(t, err)
	testutil.True(t, bytes.Equal([]byte("value-2"), got), "upsert should replace value")

	list, err := store.ListSecrets(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 1, len(list))
	testutil.Equal(t, "DUPLICATE", list[0].Name)
}

func TestStoreCreateSecretDuplicateReturnsAlreadyExists(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	testutil.NoError(t, store.CreateSecret(ctx, "DUPLICATE", []byte("value-1")))
	err := store.CreateSecret(ctx, "DUPLICATE", []byte("value-2"))
	testutil.True(t, errors.Is(err, vault.ErrSecretAlreadyExists), "duplicate create should return already exists")
}

func TestStoreUpdateSecretNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	err := store.UpdateSecret(ctx, "MISSING", []byte("value"))
	testutil.True(t, errors.Is(err, vault.ErrSecretNotFound), "update missing secret should return not found")
}

func TestStoreGetSecretNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	_, err := store.GetSecret(ctx, "MISSING")
	testutil.True(t, errors.Is(err, vault.ErrSecretNotFound), "missing secret should return not found")
}

func TestStoreDeleteSecretNotFound(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	err := store.DeleteSecret(ctx, "MISSING")
	testutil.True(t, errors.Is(err, vault.ErrSecretNotFound), "missing delete should return not found")
}

func TestStoreListSecretsNeverContainsValues(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	secretValue := "sensitive-never-list"
	testutil.NoError(t, store.SetSecret(ctx, "NO_LEAK", []byte(secretValue)))

	list, err := store.ListSecrets(ctx)
	testutil.NoError(t, err)
	encoded, err := json.Marshal(list)
	testutil.NoError(t, err)
	testutil.False(t, bytes.Contains(encoded, []byte(secretValue)), "list output must not contain secret values")
}

func TestStoreRejectsInvalidSecretNames(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	invalidNames := []string{"a/b", "..", "bad name", "line\nbreak"}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			err := store.CreateSecret(ctx, name, []byte("value"))
			testutil.True(t, errors.Is(err, vault.ErrInvalidSecretName), "create should reject invalid name %q: %v", name, err)

			err = store.SetSecret(ctx, name, []byte("value"))
			testutil.True(t, errors.Is(err, vault.ErrInvalidSecretName), "set should reject invalid name %q: %v", name, err)

			_, err = store.GetSecret(ctx, name)
			testutil.True(t, errors.Is(err, vault.ErrInvalidSecretName), "get should reject invalid name %q: %v", name, err)

			err = store.UpdateSecret(ctx, name, []byte("value"))
			testutil.True(t, errors.Is(err, vault.ErrInvalidSecretName), "update should reject invalid name %q: %v", name, err)

			err = store.DeleteSecret(ctx, name)
			testutil.True(t, errors.Is(err, vault.ErrInvalidSecretName), "delete should reject invalid name %q: %v", name, err)
		})
	}
}

func TestGetAllSecretsDecrypted(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	// Set a few secrets
	testutil.NoError(t, store.SetSecret(ctx, "API_KEY", []byte("sk-test-123")))
	testutil.NoError(t, store.SetSecret(ctx, "DB_PASSWORD", []byte("hunter2")))
	testutil.NoError(t, store.SetSecret(ctx, "EMPTY_SECRET", []byte("")))

	secrets, err := store.GetAllSecretsDecrypted(ctx)
	testutil.NoError(t, err)
	testutil.Equal(t, 3, len(secrets))
	testutil.Equal(t, "sk-test-123", secrets["API_KEY"])
	testutil.Equal(t, "hunter2", secrets["DB_PASSWORD"])
	testutil.Equal(t, "", secrets["EMPTY_SECRET"])
}

func TestGetAllSecretsDecrypted_Empty(t *testing.T) {
	ctx := context.Background()
	resetAndMigrate(t, ctx)
	store := testStore(t)

	secrets, err := store.GetAllSecretsDecrypted(ctx)
	testutil.NoError(t, err)
	testutil.NotNil(t, secrets)
	testutil.Equal(t, 0, len(secrets))
}
