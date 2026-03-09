package auth

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAppErrorSentinels(t *testing.T) {
	t.Parallel()

	testutil.True(t, ErrAppNotFound != ErrAppNameRequired, "not found != name required")
	testutil.True(t, ErrAppOwnerNotFound != ErrAppNotFound, "owner not found != app not found")
	testutil.True(t, ErrAppInvalidRateLimit != ErrAppNotFound, "invalid rate limit != app not found")
	testutil.Contains(t, ErrAppNotFound.Error(), "not found")
	testutil.Contains(t, ErrAppNameRequired.Error(), "required")
	testutil.Contains(t, ErrAppOwnerNotFound.Error(), "owner")
	testutil.Contains(t, ErrAppInvalidRateLimit.Error(), "non-negative")
}

func TestCreateAppValidation(t *testing.T) {
	t.Parallel()

	svc := newTestService()

	// Empty name should return ErrAppNameRequired before touching DB.
	_, err := svc.CreateApp(context.TODO(), "", "desc", "user-1")
	testutil.Equal(t, ErrAppNameRequired, err)
}

func TestUpdateAppValidation(t *testing.T) {
	t.Parallel()

	svc := newTestService()

	_, err := svc.UpdateApp(context.TODO(), "some-id", "", "desc", 100, 60)
	testutil.Equal(t, ErrAppNameRequired, err)
}

func TestUpdateAppNegativeRateLimitRPS(t *testing.T) {
	t.Parallel()

	svc := newTestService()

	// Service layer should reject negative rate limit values before touching DB.
	_, err := svc.UpdateApp(context.TODO(), "some-id", "My App", "desc", -1, 60)
	testutil.Equal(t, ErrAppInvalidRateLimit, err)
}

func TestUpdateAppNegativeRateLimitWindow(t *testing.T) {
	t.Parallel()

	svc := newTestService()

	_, err := svc.UpdateApp(context.TODO(), "some-id", "My App", "desc", 100, -1)
	testutil.Equal(t, ErrAppInvalidRateLimit, err)
}

func TestAppJSONSerialization(t *testing.T) {
	t.Parallel()

	app := App{
		ID:                     "550e8400-e29b-41d4-a716-446655440000",
		Name:                   "My App",
		Description:            "A test app",
		OwnerUserID:            "660e8400-e29b-41d4-a716-446655440000",
		RateLimitRPS:           100,
		RateLimitWindowSeconds: 60,
	}

	// Verify JSON field names match API contract (camelCase).
	data, err := json.Marshal(app)
	testutil.NoError(t, err)

	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))

	// These must match the JSON tags in the struct.
	testutil.Equal(t, "My App", m["name"].(string))
	testutil.Equal(t, "A test app", m["description"].(string))
	testutil.Equal(t, "660e8400-e29b-41d4-a716-446655440000", m["ownerUserId"].(string))
	testutil.Equal(t, 100.0, m["rateLimitRps"].(float64))
	testutil.Equal(t, 60.0, m["rateLimitWindowSeconds"].(float64))

	// Verify round-trip: unmarshal back to struct.
	var roundTrip App
	testutil.NoError(t, json.Unmarshal(data, &roundTrip))
	testutil.Equal(t, app.Name, roundTrip.Name)
	testutil.Equal(t, app.RateLimitRPS, roundTrip.RateLimitRPS)
}

func TestAppListResultJSONSerialization(t *testing.T) {
	t.Parallel()

	result := AppListResult{
		Items:      []App{},
		Page:       2,
		PerPage:    25,
		TotalItems: 50,
		TotalPages: 2,
	}

	data, err := json.Marshal(result)
	testutil.NoError(t, err)

	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))

	// Verify JSON field names match API contract.
	testutil.Equal(t, 2.0, m["page"].(float64))
	testutil.Equal(t, 25.0, m["perPage"].(float64))
	testutil.Equal(t, 50.0, m["totalItems"].(float64))
	testutil.Equal(t, 2.0, m["totalPages"].(float64))

	// Items should be empty array, not null.
	items, ok := m["items"].([]any)
	testutil.True(t, ok, "items should be a JSON array")
	testutil.Equal(t, 0, len(items))
}

func TestClaimsAppIDField(t *testing.T) {
	t.Parallel()

	// Verify Claims has AppID field and it serializes correctly.
	claims := Claims{
		Email: "test@example.com",
		AppID: "app-123",
	}

	data, err := json.Marshal(claims)
	testutil.NoError(t, err)

	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))
	testutil.Equal(t, "app-123", m["appId"])

	// Empty AppID should be omitted from JSON (omitempty tag).
	claimsNoApp := Claims{Email: "test@example.com"}
	data2, err := json.Marshal(claimsNoApp)
	testutil.NoError(t, err)

	var m2 map[string]any
	testutil.NoError(t, json.Unmarshal(data2, &m2))
	_, hasAppID := m2["appId"]
	testutil.True(t, !hasAppID, "appId should be omitted when empty")
}

func TestAPIKeyAppIDField(t *testing.T) {
	t.Parallel()

	// Verify APIKey has AppID field and serializes correctly.
	appID := "app-456"
	key := APIKey{
		ID:     "key-1",
		UserID: "user-1",
		Name:   "Test Key",
		AppID:  &appID,
	}

	data, err := json.Marshal(key)
	testutil.NoError(t, err)

	var m map[string]any
	testutil.NoError(t, json.Unmarshal(data, &m))
	testutil.Equal(t, "app-456", m["appId"])

	// Nil AppID should serialize as null.
	keyNoApp := APIKey{
		ID:     "key-2",
		UserID: "user-1",
		Name:   "Legacy Key",
		AppID:  nil,
	}

	data2, err := json.Marshal(keyNoApp)
	testutil.NoError(t, err)

	var m2 map[string]any
	testutil.NoError(t, json.Unmarshal(data2, &m2))
	// null is valid JSON for nil pointer.
	testutil.True(t, m2["appId"] == nil, "appId should be null for nil pointer")
}

func TestInvalidAppIDErrorSentinel(t *testing.T) {
	t.Parallel()

	// Verify ErrInvalidAppID is distinct from other errors.
	testutil.True(t, ErrInvalidAppID != ErrAPIKeyNotFound, "invalid app id != key not found")
	testutil.True(t, ErrInvalidAppID != ErrInvalidScope, "invalid app id != invalid scope")
	testutil.Contains(t, ErrInvalidAppID.Error(), "not found")
}
