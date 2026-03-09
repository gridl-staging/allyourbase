package sbmigrate

import (
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRewriteRLSExpression(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "auth.uid() to uuid",
			in:   "auth.uid() = user_id",
			want: "current_setting('ayb.user_id', true)::uuid = user_id",
		},
		{
			name: "uid() to uuid",
			in:   "uid() = user_id",
			want: "current_setting('ayb.user_id', true)::uuid = user_id",
		},
		{
			name: "auth.uid() cast to text",
			in:   "(auth.uid())::text = user_id",
			want: "current_setting('ayb.user_id', true) = user_id",
		},
		{
			name: "uid() cast to text",
			in:   "(uid())::text = user_id",
			want: "current_setting('ayb.user_id', true) = user_id",
		},
		{
			name: "auth.role()",
			in:   "auth.role() = 'authenticated'",
			want: "current_setting('ayb.user_role', true) = 'authenticated'",
		},
		{
			name: "role()",
			in:   "role() = 'authenticated'",
			want: "current_setting('ayb.user_role', true) = 'authenticated'",
		},
		{
			name: "auth.jwt() email",
			in:   "auth.jwt() ->> 'email' = email",
			want: "current_setting('ayb.user_email', true) = email",
		},
		{
			name: "jwt() email",
			in:   "jwt() ->> 'email' = email",
			want: "current_setting('ayb.user_email', true) = email",
		},
		{
			name: "auth.jwt() email no spaces",
			in:   "auth.jwt()->>'email' = email",
			want: "current_setting('ayb.user_email', true) = email",
		},
		{
			name: "multiple auth refs",
			in:   "auth.uid() = user_id AND auth.role() = 'admin'",
			want: "current_setting('ayb.user_id', true)::uuid = user_id AND current_setting('ayb.user_role', true) = 'admin'",
		},
		{
			name: "no auth refs passthrough",
			in:   "is_public = true",
			want: "is_public = true",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "complex expression",
			in:   "(auth.uid() = author_id OR is_public = true)",
			want: "(current_setting('ayb.user_id', true)::uuid = author_id OR is_public = true)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RewriteRLSExpression(tt.in)
			testutil.Equal(t, tt.want, got)
		})
	}
}

func TestGenerateRewrittenPolicy(t *testing.T) {
	t.Parallel()
	t.Run("SELECT with USING", func(t *testing.T) {
		t.Parallel()
		p := RLSPolicy{
			PolicyName: "users_select",
			TableName:  "posts",
			SchemaName: "public",
			Command:    "SELECT",
			Permissive: true,
			UsingExpr:  "auth.uid() = author_id",
		}
		got := GenerateRewrittenPolicy(p)
		want := `CREATE POLICY "users_select" ON "public"."posts" AS PERMISSIVE FOR SELECT USING (current_setting('ayb.user_id', true)::uuid = author_id);`
		testutil.Equal(t, want, got)
	})

	t.Run("INSERT with WITH CHECK", func(t *testing.T) {
		t.Parallel()
		p := RLSPolicy{
			PolicyName: "users_insert",
			TableName:  "posts",
			SchemaName: "public",
			Command:    "INSERT",
			Permissive: true,
			CheckExpr:  "auth.uid() = author_id",
		}
		got := GenerateRewrittenPolicy(p)
		want := `CREATE POLICY "users_insert" ON "public"."posts" AS PERMISSIVE FOR INSERT WITH CHECK (current_setting('ayb.user_id', true)::uuid = author_id);`
		testutil.Equal(t, want, got)
	})

	t.Run("RESTRICTIVE policy", func(t *testing.T) {
		t.Parallel()
		p := RLSPolicy{
			PolicyName: "admin_only",
			TableName:  "secrets",
			SchemaName: "public",
			Command:    "ALL",
			Permissive: false,
			UsingExpr:  "auth.role() = 'service_role'",
		}
		got := GenerateRewrittenPolicy(p)
		want := `CREATE POLICY "admin_only" ON "public"."secrets" AS RESTRICTIVE FOR ALL USING (current_setting('ayb.user_role', true) = 'service_role');`
		testutil.Equal(t, want, got)
	})

	t.Run("UPDATE with both USING and CHECK", func(t *testing.T) {
		t.Parallel()
		p := RLSPolicy{
			PolicyName: "owner_update",
			TableName:  "posts",
			SchemaName: "public",
			Command:    "UPDATE",
			Permissive: true,
			UsingExpr:  "auth.uid() = author_id",
			CheckExpr:  "auth.uid() = author_id",
		}
		got := GenerateRewrittenPolicy(p)
		want := `CREATE POLICY "owner_update" ON "public"."posts" AS PERMISSIVE FOR UPDATE USING (current_setting('ayb.user_id', true)::uuid = author_id) WITH CHECK (current_setting('ayb.user_id', true)::uuid = author_id);`
		testutil.Equal(t, want, got)
	})

	t.Run("no expressions", func(t *testing.T) {
		t.Parallel()
		p := RLSPolicy{
			PolicyName: "open_read",
			TableName:  "public_data",
			SchemaName: "public",
			Command:    "SELECT",
			Permissive: true,
		}
		got := GenerateRewrittenPolicy(p)
		want := `CREATE POLICY "open_read" ON "public"."public_data" AS PERMISSIVE FOR SELECT;`
		testutil.Equal(t, want, got)
		testutil.False(t, strings.Contains(got, "USING"), "should not have USING clause")
		testutil.False(t, strings.Contains(got, "WITH CHECK"), "should not have WITH CHECK clause")
	})
}
