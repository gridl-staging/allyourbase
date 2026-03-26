package api

import (
	"math"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestRRFMerge_BothEmpty(t *testing.T) {
	t.Parallel()
	result := rrfMerge(nil, nil, []string{"id"}, 60)
	testutil.Equal(t, 0, len(result))
}

func TestRRFMerge_FTSOnly(t *testing.T) {
	t.Parallel()
	fts := []map[string]any{
		{"id": 1, "title": "first", "_fts_rank": 0.9},
		{"id": 2, "title": "second", "_fts_rank": 0.5},
	}
	result := rrfMerge(fts, nil, []string{"id"}, 60)
	testutil.Equal(t, 2, len(result))

	// First item should be id=1 (rank 1 → score 1/61)
	testutil.Equal(t, 1, result[0]["id"])
	score0 := result[0]["_hybrid_score"].(float64)
	testutil.Equal(t, true, math.Abs(score0-1.0/61.0) < 1e-9)

	// Should have _fts_rank but no _vector_distance
	testutil.Equal(t, 0.9, result[0]["_fts_rank"])
	_, hasVD := result[0]["_vector_distance"]
	testutil.Equal(t, false, hasVD)

	// Second item
	testutil.Equal(t, 2, result[1]["id"])
	score1 := result[1]["_hybrid_score"].(float64)
	testutil.Equal(t, true, math.Abs(score1-1.0/62.0) < 1e-9)
}

func TestRRFMerge_VectorOnly(t *testing.T) {
	t.Parallel()
	vec := []map[string]any{
		{"id": 10, "title": "close", "_distance": 0.1},
		{"id": 20, "title": "far", "_distance": 0.9},
	}
	result := rrfMerge(nil, vec, []string{"id"}, 60)
	testutil.Equal(t, 2, len(result))

	// First item should be id=10 (rank 1 → score 1/61)
	testutil.Equal(t, 10, result[0]["id"])
	testutil.Equal(t, 0.1, result[0]["_vector_distance"])
	_, hasFR := result[0]["_fts_rank"]
	testutil.Equal(t, false, hasFR)
}

func TestRRFMerge_FullOverlap(t *testing.T) {
	t.Parallel()
	fts := []map[string]any{
		{"id": 1, "title": "alpha", "_fts_rank": 0.9},
		{"id": 2, "title": "beta", "_fts_rank": 0.5},
	}
	vec := []map[string]any{
		{"id": 1, "title": "alpha", "_distance": 0.1},
		{"id": 2, "title": "beta", "_distance": 0.3},
	}
	result := rrfMerge(fts, vec, []string{"id"}, 60)
	testutil.Equal(t, 2, len(result))

	// Both items appear in both lists, so each gets two RRF contributions
	// id=1: FTS rank 1 → 1/61, vector rank 1 → 1/61 = 2/61
	score0 := result[0]["_hybrid_score"].(float64)
	testutil.Equal(t, true, math.Abs(score0-2.0/61.0) < 1e-9)
	testutil.Equal(t, 0.9, result[0]["_fts_rank"])
	testutil.Equal(t, 0.1, result[0]["_vector_distance"])

	// id=2: FTS rank 2 → 1/62, vector rank 2 → 1/62 = 2/62
	score1 := result[1]["_hybrid_score"].(float64)
	testutil.Equal(t, true, math.Abs(score1-2.0/62.0) < 1e-9)
	testutil.Equal(t, 0.5, result[1]["_fts_rank"])
	testutil.Equal(t, 0.3, result[1]["_vector_distance"])
}

func TestRRFMerge_PartialOverlap(t *testing.T) {
	t.Parallel()
	fts := []map[string]any{
		{"id": 1, "title": "shared", "_fts_rank": 0.9},
		{"id": 2, "title": "fts-only", "_fts_rank": 0.5},
	}
	vec := []map[string]any{
		{"id": 1, "title": "shared", "_distance": 0.1},
		{"id": 3, "title": "vec-only", "_distance": 0.2},
	}
	result := rrfMerge(fts, vec, []string{"id"}, 60)
	testutil.Equal(t, 3, len(result))

	// id=1 appears in both → score = 1/61 + 1/61 = 2/61 ≈ 0.0328 (highest)
	testutil.Equal(t, 1, result[0]["id"])
	score0 := result[0]["_hybrid_score"].(float64)
	testutil.Equal(t, true, math.Abs(score0-2.0/61.0) < 1e-9)
	testutil.Equal(t, 0.9, result[0]["_fts_rank"])
	testutil.Equal(t, 0.1, result[0]["_vector_distance"])

	// id=2 and id=3 both have single-signal scores: 1/62 ≈ 0.0161
	// Tie-breaking by PK ascending → id=2 before id=3
	testutil.Equal(t, 2, result[1]["id"])
	_, hasVD := result[1]["_vector_distance"]
	testutil.Equal(t, false, hasVD)

	testutil.Equal(t, 3, result[2]["id"])
	_, hasFR := result[2]["_fts_rank"]
	testutil.Equal(t, false, hasFR)
}

func TestRRFMerge_Disjoint(t *testing.T) {
	t.Parallel()
	fts := []map[string]any{
		{"id": 1, "title": "fts1", "_fts_rank": 0.9},
		{"id": 2, "title": "fts2", "_fts_rank": 0.5},
	}
	vec := []map[string]any{
		{"id": 3, "title": "vec1", "_distance": 0.1},
		{"id": 4, "title": "vec2", "_distance": 0.2},
	}
	result := rrfMerge(fts, vec, []string{"id"}, 60)
	testutil.Equal(t, 4, len(result))

	// All items have equal scores at each rank position
	// Rank 1 items: id=1 (fts) and id=3 (vec), both score 1/61
	// Rank 2 items: id=2 (fts) and id=4 (vec), both score 1/62
	// Tie-breaking by PK ascending: 1, 3, 2, 4
	testutil.Equal(t, 1, result[0]["id"])
	testutil.Equal(t, 3, result[1]["id"])
	testutil.Equal(t, 2, result[2]["id"])
	testutil.Equal(t, 4, result[3]["id"])
}

func TestRRFMerge_TieBreakingByPK(t *testing.T) {
	t.Parallel()
	// Both at rank 1 in their respective lists → same RRF score
	fts := []map[string]any{
		{"id": 5, "title": "higher-pk", "_fts_rank": 0.9},
	}
	vec := []map[string]any{
		{"id": 2, "title": "lower-pk", "_distance": 0.1},
	}
	result := rrfMerge(fts, vec, []string{"id"}, 60)
	testutil.Equal(t, 2, len(result))
	// id=2 comes before id=5 by ascending PK
	testutil.Equal(t, 2, result[0]["id"])
	testutil.Equal(t, 5, result[1]["id"])
}

func TestRRFMerge_CompositePK(t *testing.T) {
	t.Parallel()
	fts := []map[string]any{
		{"org_id": 1, "user_id": 10, "name": "alice", "_fts_rank": 0.9},
		{"org_id": 1, "user_id": 20, "name": "bob", "_fts_rank": 0.5},
	}
	vec := []map[string]any{
		{"org_id": 1, "user_id": 10, "name": "alice", "_distance": 0.1},
		{"org_id": 2, "user_id": 5, "name": "carol", "_distance": 0.2},
	}
	result := rrfMerge(fts, vec, []string{"org_id", "user_id"}, 60)
	testutil.Equal(t, 3, len(result))

	// alice (org_id=1, user_id=10) appears in both → highest score
	testutil.Equal(t, 1, result[0]["org_id"])
	testutil.Equal(t, 10, result[0]["user_id"])
	testutil.Equal(t, 0.9, result[0]["_fts_rank"])
	testutil.Equal(t, 0.1, result[0]["_vector_distance"])
}
