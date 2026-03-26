package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestAIUsageAccountingMigrationSQLLoadable(t *testing.T) {
	t.Parallel()

	b, err := fs.ReadFile(embeddedMigrations, "sql/122_ayb_ai_usage_accounting.sql")
	testutil.NoError(t, err)
	sql := string(b)

	testutil.True(t, strings.Contains(sql, "ADD COLUMN IF NOT EXISTS cost_usd"), "122 must add cost_usd to call log")
	testutil.True(t, strings.Contains(sql, "_ayb_ai_usage_daily"), "122 must create daily usage table")
	testutil.True(t, strings.Contains(sql, "UNIQUE (day, provider, model)"), "122 must enforce daily uniqueness")
	testutil.True(t, strings.Contains(sql, "idx_ayb_ai_usage_daily_provider_model_day"), "122 must include provider/model/day index")
}
