package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/allyourbase/ayb/internal/testutil"
)

func TestOrgColumnsWithAlias(t *testing.T) {
	t.Parallel()

	testutil.Equal(
		t,
		"o.id, o.name, o.slug, o.parent_org_id, o.plan_tier, o.created_at, o.updated_at",
		orgColumnsWithAlias("o"),
	)
}

func TestValidateParentOrgCycleCreateSkipsGraphWalk(t *testing.T) {
	t.Parallel()

	store := &PostgresOrgStore{}
	parentOrgID := "11111111-1111-1111-1111-111111111111"

	err := store.validateParentOrgCycle(context.Background(), "", &parentOrgID)
	testutil.NoError(t, err)
}

func TestValidateParentOrgCycleRejectsSelfReference(t *testing.T) {
	t.Parallel()

	store := &PostgresOrgStore{}
	parentOrgID := "11111111-1111-1111-1111-111111111111"

	err := store.validateParentOrgCycle(context.Background(), parentOrgID, &parentOrgID)
	testutil.True(t, errors.Is(err, ErrCircularParentOrg))
}
