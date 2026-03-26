package graphql

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGraphQLSplitFilesHaveNoStaleBoundaryComments(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve test file path")
	}
	graphqlDir := filepath.Dir(currentFile)

	tests := []struct {
		name      string
		file      string
		forbidden string
	}{
		{
			name:      "subscriptions has no leftover where comment",
			file:      "subscriptions.go",
			forbidden: "matchesGraphQLWhere checks if a row satisfies a GraphQL where clause",
		},
		{
			name:      "where has no leftover servehttp comment",
			file:      "where.go",
			forbidden: "ServeHTTP serves GraphQL queries over HTTP and WebSocket",
		},
		{
			name:      "mutation sql has no leftover delete resolver comment",
			file:      "mutation_sql.go",
			forbidden: "resolveDeleteMutation executes a DELETE mutation",
		},
		{
			name:      "resolve mutations has no leftover build select comment",
			file:      "resolve_mutations.go",
			forbidden: "buildSelectStatement generates a SELECT * FROM table WHERE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			contents, err := os.ReadFile(filepath.Join(graphqlDir, tc.file))
			if err != nil {
				t.Fatalf("read %s: %v", tc.file, err)
			}
			if strings.Contains(string(contents), tc.forbidden) {
				t.Fatalf("%s still contains stale boundary comment text %q", tc.file, tc.forbidden)
			}
		})
	}
}
