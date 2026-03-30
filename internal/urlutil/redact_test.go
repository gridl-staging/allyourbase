package urlutil

import (
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRedactURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "strips user and password", input: "postgres://user:secret@host:5432/mydb", want: "postgres://***@host:5432/mydb"},
		{name: "strips user only", input: "postgres://admin@host:5432/db", want: "postgres://***@host:5432/db"},
		{name: "keeps no userinfo", input: "postgres://host:5432/db", want: "postgres://host:5432/db"},
		{name: "preserves query params", input: "postgres://user:pass@host/db?sslmode=require", want: "postgres://***@host/db?sslmode=require"},
		{name: "preserves fragment", input: "https://user:pass@host/path#frag", want: "https://***@host/path#frag"},
		{name: "empty string", input: "", want: ""},
		{name: "plain hostname", input: "localhost", want: "localhost"},
		{name: "http URL no creds", input: "http://example.com/api", want: "http://example.com/api"},
		{name: "password with special chars", input: "postgres://user:p%40ss%23word@host/db", want: "postgres://***@host/db"},
		{name: "parse error returns stars", input: "://not a valid url", want: "***"},
		{name: "invalid escapes return stars", input: "not a url %%", want: "***"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RedactURL(tt.input)
			testutil.Equal(t, tt.want, got)
		})
	}
}
