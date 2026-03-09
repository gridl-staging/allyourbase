package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

// --- Validation: empty, too many, bad method ---

func TestBatchEmptyOperations(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/batch", `{"operations":[]}`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "operations array is empty")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#batch-operations")
}

func TestBatchTooManyOperations(t *testing.T) {
	// Build a request with maxBatchSize+1 operations.
	t.Parallel()

	ops := make([]BatchOperation, maxBatchSize+1)
	for i := range ops {
		ops[i] = BatchOperation{Method: "create", Body: map[string]any{"email": "a@b.com"}}
	}
	body, _ := json.Marshal(BatchRequest{Operations: ops})

	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/batch", string(body))
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "too many operations")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#batch-operations")
}

func TestBatchInvalidJSON(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/batch", `{bad`)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "invalid JSON body")
}

func TestBatchUnknownMethod(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"upsert","body":{"email":"a@b.com"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "unknown method")
	testutil.Contains(t, resp.DocURL, "/guide/api-reference#batch-operations")
}

// --- Validation: create ---

func TestBatchCreateMissingBody(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"create"}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "create requires a body")
}

func TestBatchCreateNoRecognizedColumns(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"create","body":{"unknown":"val"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no recognized columns")
}

// --- Validation: update ---

func TestBatchUpdateMissingID(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"update","body":{"email":"a@b.com"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "update requires an id")
}

func TestBatchUpdateMissingBody(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"update","id":"123"}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "update requires a body")
}

func TestBatchUpdateNoRecognizedColumns(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"update","id":"123","body":{"unknown":"val"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no recognized columns")
}

// --- Validation: delete ---

func TestBatchDeleteMissingID(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"delete"}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "delete requires an id")
}

// --- Collection guards ---

func TestBatchCollectionNotFound(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"create","body":{"data":"x"}}]}`
	w := doRequest(h, "POST", "/collections/nonexistent/batch", body)
	testutil.Equal(t, http.StatusNotFound, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "collection not found")
}

func TestBatchOnViewNotAllowed(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"create","body":{"message":"x"}}]}`
	w := doRequest(h, "POST", "/collections/logs/batch", body)
	testutil.Equal(t, http.StatusMethodNotAllowed, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "write operations not allowed")
}

func TestBatchNoPrimaryKey(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"create","body":{"data":"x"}}]}`
	w := doRequest(h, "POST", "/collections/nopk/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "no primary key")
}

func TestBatchSchemaCacheNotReady(t *testing.T) {
	t.Parallel()
	h := testHandler(nil)
	body := `{"operations":[{"method":"create","body":{"email":"a@b.com"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusServiceUnavailable, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "schema cache not ready")
}

func TestBatchExactlyMaxBatchSizePassesSizeCheck(t *testing.T) {
	// Verify maxBatchSize ops passes the size guard but maxBatchSize+1 does not.
	// We use an invalid method so validation fails AFTER the size check,
	// confirming the size check itself accepted maxBatchSize.
	t.Parallel()

	ops := make([]BatchOperation, maxBatchSize)
	for i := range ops {
		ops[i] = BatchOperation{Method: "create", Body: map[string]any{"email": "a@b.com"}}
	}
	// Make last op invalid so we get a validation error, not a DB panic.
	ops[maxBatchSize-1] = BatchOperation{Method: "nope"}
	body, _ := json.Marshal(BatchRequest{Operations: ops})

	h := testHandler(testSchema())
	w := doRequest(h, "POST", "/collections/users/batch", string(body))
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	// Should fail on "unknown method", NOT on "too many operations".
	testutil.Contains(t, resp.Message, "unknown method")
}

func TestBatchErrorIncludesIndexZero(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	body := `{"operations":[{"method":"nope","body":{"email":"a@b.com"}}]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "operation[0]")
}

// --- validateBatchOp unit tests ---

func TestValidateBatchOp(t *testing.T) {
	t.Parallel()
	sc := testSchema()
	tbl := sc.TableByName("users")

	tests := []struct {
		name    string
		op      BatchOperation
		wantErr string
	}{
		{
			name: "valid create",
			op:   BatchOperation{Method: "create", Body: map[string]any{"email": "a@b.com"}},
		},
		{
			name: "valid update",
			op:   BatchOperation{Method: "update", ID: "123", Body: map[string]any{"email": "a@b.com"}},
		},
		{
			name: "valid delete",
			op:   BatchOperation{Method: "delete", ID: "123"},
		},
		{
			name:    "empty method",
			op:      BatchOperation{},
			wantErr: "unknown method",
		},
		{
			name:    "create empty body",
			op:      BatchOperation{Method: "create", Body: map[string]any{}},
			wantErr: "create requires a body",
		},
		{
			name:    "update no id",
			op:      BatchOperation{Method: "update", Body: map[string]any{"email": "x"}},
			wantErr: "update requires an id",
		},
		{
			name:    "update no body",
			op:      BatchOperation{Method: "update", ID: "123"},
			wantErr: "update requires a body",
		},
		{
			name:    "delete no id",
			op:      BatchOperation{Method: "delete"},
			wantErr: "delete requires an id",
		},
		{
			name:    "bad method",
			op:      BatchOperation{Method: "merge"},
			wantErr: "unknown method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBatchOp(tbl, tt.op)
			if tt.wantErr == "" {
				testutil.NoError(t, err)
			} else {
				testutil.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

// --- Validation error includes operation index ---

func TestBatchErrorIncludesIndex(t *testing.T) {
	t.Parallel()
	h := testHandler(testSchema())
	// First op is valid, second has bad method — error should say "operation[1]".
	body := `{"operations":[
		{"method":"create","body":{"email":"a@b.com"}},
		{"method":"nope","body":{"email":"a@b.com"}}
	]}`
	w := doRequest(h, "POST", "/collections/users/batch", body)
	testutil.Equal(t, http.StatusBadRequest, w.Code)
	resp := decodeError(t, w)
	testutil.Contains(t, resp.Message, "operation[1]")
}

// maxBatchSize enforcement is covered by TestBatchTooManyOperations
// and TestBatchExactlyMaxBatchSizePassesSizeCheck.
