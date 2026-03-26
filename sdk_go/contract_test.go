package allyourbase

import (
	"encoding/json"
	"testing"
)

func TestContractAuthResponseJSONShape(t *testing.T) {
	raw := []byte(`{"token":"jwt_stage3","refreshToken":"refresh_stage3","user":{"id":"usr_1","email":"dev@allyourbase.io","email_verified":true,"created_at":"2026-01-01T00:00:00Z","updated_at":null}}`)
	var out AuthResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Token != "jwt_stage3" || out.RefreshToken != "refresh_stage3" || out.User.ID != "usr_1" {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.User.Email != "dev@allyourbase.io" {
		t.Fatalf("bad email parse: %+v", out.User)
	}
	if out.User.EmailVerified == nil || *out.User.EmailVerified != true {
		t.Fatalf("bad email verified parse: %+v", out.User)
	}
	if out.User.CreatedAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("bad createdAt parse: %+v", out.User)
	}
	if out.User.UpdatedAt != nil {
		t.Fatalf("expected nil updatedAt, got: %+v", out.User.UpdatedAt)
	}
}

func TestContractListResponseJSONShape(t *testing.T) {
	raw := []byte(`{"items":[{"id":"rec_1","title":"First"},{"id":"rec_2","title":"Second"}],"page":1,"perPage":2,"totalItems":2,"totalPages":1}`)
	var out ListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.PerPage != 2 || len(out.Items) != 2 {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.Page != 1 || out.TotalItems != 2 || out.TotalPages != 1 {
		t.Fatalf("bad metadata parse: %+v", out)
	}
	if out.Items[0]["title"] != "First" || out.Items[1]["title"] != "Second" {
		t.Fatalf("bad item order parse: %+v", out.Items)
	}
}

func TestContractStorageObjectJSONShape(t *testing.T) {
	raw := []byte(`{"id":"file_abc123","bucket":"uploads","name":"document.pdf","size":1024,"contentType":"application/pdf","userId":"usr_1","createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-02T12:30:00Z"}`)
	var out StorageObject
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.ContentType != "application/pdf" || out.Name != "document.pdf" || out.Bucket != "uploads" {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.UserID == nil || *out.UserID != "usr_1" {
		t.Fatalf("bad userId parse: %+v", out)
	}
	if out.UpdatedAt == nil || *out.UpdatedAt != "2026-01-02T12:30:00Z" {
		t.Fatalf("bad updatedAt parse: %+v", out)
	}
}

func TestContractStorageListResponseJSONShape(t *testing.T) {
	raw := []byte(`{"items":[{"id":"file_1","bucket":"uploads","name":"doc1.pdf","size":1024,"contentType":"application/pdf","userId":"usr_1","createdAt":"2026-01-01T00:00:00Z","updatedAt":null},{"id":"file_2","bucket":"uploads","name":"image.png","size":2048,"contentType":"image/png","userId":null,"createdAt":"2026-01-02T00:00:00Z","updatedAt":null}],"totalItems":2}`)
	var out StorageListResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.TotalItems != 2 || len(out.Items) != 2 {
		t.Fatalf("bad parse: %+v", out)
	}
	if out.Items[0].UserID == nil || *out.Items[0].UserID != "usr_1" {
		t.Fatalf("bad first userId parse: %+v", out.Items[0])
	}
	if out.Items[1].UserID != nil {
		t.Fatalf("expected nil second userId, got: %+v", out.Items[1].UserID)
	}
	if out.Items[0].UpdatedAt != nil || out.Items[1].UpdatedAt != nil {
		t.Fatalf("expected nil updatedAt for list fixtures: %+v", out.Items)
	}
}

func TestContractErrorResponseNumericCodeShape(t *testing.T) {
	raw := []byte(`{"code":403,"message":"forbidden","data":{"resource":"posts"},"doc_url":"https://allyourbase.io/docs/errors#forbidden"}`)
	err := normalizeError(403, "Forbidden", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "403" || apiErr.Message != "forbidden" {
		t.Fatalf("bad parse: %+v", apiErr)
	}
	if apiErr.Data["resource"] != "posts" || apiErr.DocURL != "https://allyourbase.io/docs/errors#forbidden" {
		t.Fatalf("bad details parse: %+v", apiErr)
	}
}

func TestContractErrorResponseNumericCodePreservesNonIntegerValue(t *testing.T) {
	raw := []byte(`{"code":403.5,"message":"forbidden"}`)
	err := normalizeError(403, "Forbidden", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "403.5" {
		t.Fatalf("expected fractional code to be preserved, got: %q", apiErr.Code)
	}
}

func TestContractErrorResponseStringCodeShape(t *testing.T) {
	raw := []byte(`{"code":"auth/missing-refresh-token","message":"Missing refresh token","data":{"detail":"refresh token not available"}}`)
	err := normalizeError(400, "Bad Request", raw)
	apiErr, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T", err)
	}
	if apiErr.Code != "auth/missing-refresh-token" || apiErr.Message != "Missing refresh token" {
		t.Fatalf("bad parse: %+v", apiErr)
	}
	if apiErr.Data["detail"] != "refresh token not available" {
		t.Fatalf("bad details parse: %+v", apiErr)
	}
}
