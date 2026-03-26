// Package schema Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/internal/schema/srid.go.
package schema

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var sridLookupCache sync.Map

type sridLookupCacheKey struct {
	poolConnString string
	srid           int
}

type SRIDInfo struct {
	AuthName    string
	AuthSRID    int
	Name        string
	Description string
}

// TODO: Document LookupSRID.
func LookupSRID(ctx context.Context, pool *pgxpool.Pool, srid int) (*SRIDInfo, error) {
	cacheKey := newSRIDLookupCacheKey(pool, srid)
	if cached, ok := sridLookupCache.Load(cacheKey); ok {
		if info, ok := cached.(*SRIDInfo); ok && info != nil {
			return info, nil
		}
	}

	if pool == nil {
		return nil, fmt.Errorf("nil pool for uncached srid lookup: %d", srid)
	}

	var authName string
	var authSRID int
	var srtext string
	err := pool.QueryRow(ctx, `SELECT auth_name, auth_srid, srtext FROM spatial_ref_sys WHERE srid = $1`, srid).
		Scan(&authName, &authSRID, &srtext)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("srid not found: %d", srid)
	}
	if err != nil {
		return nil, fmt.Errorf("querying srid %d: %w", srid, err)
	}

	info := &SRIDInfo{
		AuthName:    authName,
		AuthSRID:    authSRID,
		Name:        fmt.Sprintf("%s:%d", authName, authSRID),
		Description: parseSRTextDescription(srtext),
	}
	sridLookupCache.Store(cacheKey, info)
	return info, nil
}

func newSRIDLookupCacheKey(pool *pgxpool.Pool, srid int) sridLookupCacheKey {
	if pool == nil {
		return sridLookupCacheKey{srid: srid}
	}

	return sridLookupCacheKey{
		poolConnString: pool.Config().ConnString(),
		srid:           srid,
	}
}

func parseSRTextDescription(srtext string) string {
	trimmed := strings.TrimSpace(srtext)
	firstQuote := strings.IndexByte(trimmed, '"')
	if firstQuote == -1 || firstQuote+1 >= len(trimmed) {
		return trimmed
	}
	remainder := trimmed[firstQuote+1:]
	secondQuote := strings.IndexByte(remainder, '"')
	if secondQuote == -1 {
		return trimmed
	}
	return strings.TrimSpace(remainder[:secondQuote])
}
