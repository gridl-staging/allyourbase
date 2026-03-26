package config

import "testing"

func TestGraphQLDefaults(t *testing.T) {
	t.Parallel()
	cfg := Default()
	if cfg.GraphQL.MaxDepth != 0 {
		t.Fatalf("expected GraphQL.MaxDepth default 0, got %d", cfg.GraphQL.MaxDepth)
	}
	if cfg.GraphQL.MaxComplexity != 0 {
		t.Fatalf("expected GraphQL.MaxComplexity default 0, got %d", cfg.GraphQL.MaxComplexity)
	}
	if cfg.GraphQL.Introspection != "" {
		t.Fatalf("expected GraphQL.Introspection default empty string, got %q", cfg.GraphQL.Introspection)
	}
}

func TestGraphQLIntrospectionValidation(t *testing.T) {
	t.Parallel()
	cfg := Default()
	cfg.GraphQL.Introspection = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected validation error for invalid graphql.introspection")
	}
}
