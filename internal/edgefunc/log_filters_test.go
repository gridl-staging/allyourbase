package edgefunc

import (
	"strings"
	"testing"
	"time"

	"github.com/allyourbase/ayb/internal/testutil"
	"github.com/google/uuid"
)

func TestNormalizeLogListOptions_DefaultsAndCap(t *testing.T) {
	since := time.Date(2026, 2, 24, 8, 0, 0, 0, time.UTC)
	until := since.Add(1 * time.Hour)

	opts, err := normalizeLogListOptions(LogListOptions{
		PerPage:     5001,
		Status:      "success",
		TriggerType: string(TriggerHTTP),
		Since:       &since,
		Until:       &until,
	})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, opts.Page)
	testutil.Equal(t, 1000, opts.PerPage)
	testutil.Equal(t, "success", opts.Status)
	testutil.Equal(t, string(TriggerHTTP), opts.TriggerType)
}

func TestNormalizeLogListOptions_InvalidStatus(t *testing.T) {
	_, err := normalizeLogListOptions(LogListOptions{Status: "maybe"})
	testutil.True(t, err != nil, "expected invalid status error")
}

func TestNormalizeLogListOptions_InvalidTriggerType(t *testing.T) {
	_, err := normalizeLogListOptions(LogListOptions{TriggerType: "queue"})
	testutil.True(t, err != nil, "expected invalid trigger type error")
}

func TestNormalizeLogListOptions_InvalidSinceUntilRange(t *testing.T) {
	since := time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC)
	until := since.Add(-1 * time.Hour)

	_, err := normalizeLogListOptions(LogListOptions{
		Since: &since,
		Until: &until,
	})
	testutil.True(t, err != nil, "expected invalid since/until range error")
}

func TestNormalizeLogListOptions_InvalidNegativePage(t *testing.T) {
	_, err := normalizeLogListOptions(LogListOptions{Page: -1})
	testutil.True(t, err != nil, "expected invalid negative page error")
}

func TestNormalizeLogListOptions_InvalidNegativePerPage(t *testing.T) {
	_, err := normalizeLogListOptions(LogListOptions{PerPage: -10})
	testutil.True(t, err != nil, "expected invalid negative perPage error")
}

func TestNormalizeLogListOptions_ZeroPerPageDefaultsTo50(t *testing.T) {
	opts, err := normalizeLogListOptions(LogListOptions{Page: 1, PerPage: 0})
	testutil.NoError(t, err)
	testutil.Equal(t, 50, opts.PerPage)
	testutil.Equal(t, 1, opts.Page)
}

func TestNormalizeLogListOptions_ZeroPageDefaultsTo1(t *testing.T) {
	opts, err := normalizeLogListOptions(LogListOptions{Page: 0, PerPage: 25})
	testutil.NoError(t, err)
	testutil.Equal(t, 1, opts.Page)
	testutil.Equal(t, 25, opts.PerPage)
}

func TestBuildListByFunctionQuery_NoFilters(t *testing.T) {
	functionID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	query, args := buildListByFunctionQuery(functionID, LogListOptions{
		Page:    1,
		PerPage: 50,
	})

	testutil.True(t, strings.Contains(query, "WHERE function_id = $1"), "missing function_id predicate")
	testutil.True(t, !strings.Contains(query, "status ="), "should not have status predicate")
	testutil.True(t, !strings.Contains(query, "trigger_type ="), "should not have trigger_type predicate")
	testutil.True(t, !strings.Contains(query, "created_at >="), "should not have since predicate")
	testutil.True(t, !strings.Contains(query, "created_at <="), "should not have until predicate")
	testutil.True(t, strings.Contains(query, "LIMIT $2 OFFSET $3"), "missing pagination predicates")
	testutil.Equal(t, 3, len(args))

	argFunctionID, ok := args[0].(uuid.UUID)
	testutil.True(t, ok, "arg 0 should be a UUID")
	testutil.Equal(t, functionID, argFunctionID)

	argLimit, ok := args[1].(int)
	testutil.True(t, ok, "arg 1 should be an int limit")
	testutil.Equal(t, 50, argLimit)

	argOffset, ok := args[2].(int)
	testutil.True(t, ok, "arg 2 should be an int offset")
	testutil.Equal(t, 0, argOffset)
}

func TestBuildListByFunctionQuery_StatusFilterOnly(t *testing.T) {
	functionID := uuid.MustParse("33333333-3333-3333-3333-333333333333")

	query, args := buildListByFunctionQuery(functionID, LogListOptions{
		Page:    1,
		PerPage: 20,
		Status:  "success",
	})

	testutil.True(t, strings.Contains(query, "WHERE function_id = $1 AND status = $2"), "missing status predicate")
	testutil.True(t, !strings.Contains(query, "trigger_type ="), "should not have trigger_type predicate")
	testutil.True(t, strings.Contains(query, "LIMIT $3 OFFSET $4"), "missing pagination predicates")
	testutil.Equal(t, 4, len(args))
	testutil.Equal(t, "success", args[1].(string))
}

func TestBuildListByFunctionQuery_IncludesFiltersAndPagination(t *testing.T) {
	functionID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	since := time.Date(2026, 2, 24, 8, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 24, 9, 0, 0, 0, time.UTC)

	query, args := buildListByFunctionQuery(functionID, LogListOptions{
		Page:        2,
		PerPage:     20,
		Status:      "error",
		TriggerType: string(TriggerCron),
		Since:       &since,
		Until:       &until,
	})

	testutil.True(t, strings.Contains(query, "WHERE function_id = $1"), "missing function_id predicate")
	testutil.True(t, strings.Contains(query, "status = $2"), "missing status predicate")
	testutil.True(t, strings.Contains(query, "trigger_type = $3"), "missing trigger_type predicate")
	testutil.True(t, strings.Contains(query, "created_at >= $4"), "missing since predicate")
	testutil.True(t, strings.Contains(query, "created_at <= $5"), "missing until predicate")
	testutil.True(t, strings.Contains(query, "LIMIT $6 OFFSET $7"), "missing pagination predicates")

	testutil.Equal(t, 7, len(args))

	argFunctionID, ok := args[0].(uuid.UUID)
	testutil.True(t, ok, "arg 0 should be a UUID")
	testutil.Equal(t, functionID, argFunctionID)

	argStatus, ok := args[1].(string)
	testutil.True(t, ok, "arg 1 should be a status string")
	testutil.Equal(t, "error", argStatus)

	argTriggerType, ok := args[2].(string)
	testutil.True(t, ok, "arg 2 should be a trigger_type string")
	testutil.Equal(t, string(TriggerCron), argTriggerType)

	argSince, ok := args[3].(time.Time)
	testutil.True(t, ok, "arg 3 should be a time.Time")
	testutil.Equal(t, since, argSince)

	argUntil, ok := args[4].(time.Time)
	testutil.True(t, ok, "arg 4 should be a time.Time")
	testutil.Equal(t, until, argUntil)

	argLimit, ok := args[5].(int)
	testutil.True(t, ok, "arg 5 should be an int limit")
	testutil.Equal(t, 20, argLimit)

	argOffset, ok := args[6].(int)
	testutil.True(t, ok, "arg 6 should be an int offset")
	testutil.Equal(t, 20, argOffset)
}
