package tenant

// normalizeIntLimit converts an optional *int limit to a normalized int64 value.
// Returns (0, false) if the limit is nil or non-positive.
func normalizeIntLimit(limit *int) (int64, bool) {
	if limit == nil || *limit <= 0 {
		return 0, false
	}
	return int64(*limit), true
}
