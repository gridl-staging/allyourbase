package server

import "testing"

func TestBuildPathLikeClauseEscapesLiteralLikeChars(t *testing.T) {
	t.Parallel()

	clause, arg, next := buildPathLikeClause("/api/user_profiles%v1", 3)
	if clause != "path LIKE $3 ESCAPE '\\'" {
		t.Fatalf("unexpected clause: %q", clause)
	}
	if arg != "/api/user\\_profiles\\%v1" {
		t.Fatalf("unexpected arg: %q", arg)
	}
	if next != 4 {
		t.Fatalf("unexpected next arg pos: %d", next)
	}
}

func TestBuildPathLikeClauseSupportsStarWildcard(t *testing.T) {
	t.Parallel()

	clause, arg, next := buildPathLikeClause("/api/collections/*", 1)
	if clause != "path LIKE $1 ESCAPE '\\'" {
		t.Fatalf("unexpected clause: %q", clause)
	}
	if arg != "/api/collections/%" {
		t.Fatalf("unexpected arg: %q", arg)
	}
	if next != 2 {
		t.Fatalf("unexpected next arg pos: %d", next)
	}
}
