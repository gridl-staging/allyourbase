package auth

import (
	"context"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/golang-jwt/jwt/v5"
)

func TestSetRLSContextNilClaims(t *testing.T) {
	// Nil claims should be a no-op.
	t.Parallel()

	err := SetRLSContext(context.Background(), nil, nil)
	testutil.NoError(t, err)
}

func TestEscapeLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single quote becomes doubled",
			input: "test'value",
			want:  "test''value",
		},
		{
			name:  "multiple single quotes",
			input: "it's a test's value",
			want:  "it''s a test''s value",
		},
		{
			name:  "SQL injection attempt",
			input: "'; DROP TABLE users; --",
			want:  "''; DROP TABLE users; --",
		},
		{
			name:  "no quotes unchanged",
			input: "normalvalue",
			want:  "normalvalue",
		},
		{
			name:  "empty string unchanged",
			input: "",
			want:  "",
		},
		{
			name:  "backslash preserved",
			input: `test\value`,
			want:  `test\value`,
		},
		{
			name:  "newline preserved",
			input: "test\nvalue",
			want:  "test\nvalue",
		},
		{
			name:  "null byte preserved",
			input: "test\x00value",
			want:  "test\x00value",
		},
		{
			name:  "multiple escape attempts",
			input: `'; DROP TABLE users; --' OR '1'='1`,
			want:  `''; DROP TABLE users; --'' OR ''1''=''1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := escapeLiteral(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestRLSStatements(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		userID     string
		email      string
		wantRole   string
		wantUserID string
		wantEmail  string
	}{
		{
			name:       "normal values",
			userID:     "user-123",
			email:      "test@example.com",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = 'user-123'",
			wantEmail:  "SET LOCAL ayb.user_email = 'test@example.com'",
		},
		{
			name:       "single quotes in user_id",
			userID:     "user'123",
			email:      "test@example.com",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = 'user''123'",
			wantEmail:  "SET LOCAL ayb.user_email = 'test@example.com'",
		},
		{
			name:       "single quotes in email",
			userID:     "user-123",
			email:      "test'user@example.com",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = 'user-123'",
			wantEmail:  "SET LOCAL ayb.user_email = 'test''user@example.com'",
		},
		{
			name:       "SQL injection in user_id",
			userID:     "'; DROP TABLE users; --",
			email:      "test@example.com",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = '''; DROP TABLE users; --'",
			wantEmail:  "SET LOCAL ayb.user_email = 'test@example.com'",
		},
		{
			name:       "SQL injection in email",
			userID:     "user-123",
			email:      "hacker'; DELETE FROM auth.users; --@evil.com",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = 'user-123'",
			wantEmail:  "SET LOCAL ayb.user_email = 'hacker''; DELETE FROM auth.users; --@evil.com'",
		},
		{
			name:       "empty values",
			userID:     "",
			email:      "",
			wantRole:   `SET LOCAL ROLE "ayb_authenticated"`,
			wantUserID: "SET LOCAL ayb.user_id = ''",
			wantEmail:  "SET LOCAL ayb.user_email = ''",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			claims := &Claims{
				RegisteredClaims: jwt.RegisteredClaims{Subject: tt.userID},
				Email:            tt.email,
			}
			stmts := rlsStatements(claims)
			testutil.Equal(t, tt.wantRole, stmts[0])
			testutil.Equal(t, tt.wantUserID, stmts[1])
			testutil.Equal(t, tt.wantEmail, stmts[2])
			testutil.Equal(t, 3, len(stmts))
		})
	}
}

func TestRLSStatements_TenantID(t *testing.T) {
	t.Parallel()

	t.Run("tenant_id set", func(t *testing.T) {
		t.Parallel()
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@b.com",
			TenantID:         "tenant-abc-123",
		}
		stmts := rlsStatements(claims)
		testutil.Equal(t, 4, len(stmts))
		testutil.Equal(t, "SET LOCAL ayb.tenant_id = 'tenant-abc-123'", stmts[3])
	})

	t.Run("tenant_id empty omitted", func(t *testing.T) {
		t.Parallel()
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@b.com",
			TenantID:         "",
		}
		stmts := rlsStatements(claims)
		testutil.Equal(t, 3, len(stmts))
	})

	t.Run("tenant_id whitespace omitted", func(t *testing.T) {
		t.Parallel()
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@b.com",
			TenantID:         "   ",
		}
		stmts := rlsStatements(claims)
		testutil.Equal(t, 3, len(stmts))
	})

	t.Run("tenant_id with special chars escaped", func(t *testing.T) {
		t.Parallel()
		claims := &Claims{
			RegisteredClaims: jwt.RegisteredClaims{Subject: "user-1"},
			Email:            "a@b.com",
			TenantID:         "tenant'; DROP TABLE tenants; --",
		}
		stmts := rlsStatements(claims)
		testutil.Equal(t, 4, len(stmts))
		testutil.Equal(t, "SET LOCAL ayb.tenant_id = 'tenant''; DROP TABLE tenants; --'", stmts[3])
	})
}
