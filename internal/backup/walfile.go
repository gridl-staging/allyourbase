package backup

import (
	"fmt"
	"strconv"
	"strings"
)

const walSegmentSizeBytes uint64 = 16 * 1024 * 1024

// WALFileName is a parsed PostgreSQL WAL segment filename.
// Format: TTTTTTTTSSSSSSSSLLLLLLLL (24 hex chars).
type WALFileName struct {
	Timeline     int
	SegmentHigh  uint32
	SegmentLow   uint32
	OriginalName string
}

// ParseWALFileName validates and parses a WAL filename.
func ParseWALFileName(name string) (*WALFileName, error) {
	if name == "" {
		return nil, fmt.Errorf("invalid WAL filename: empty string")
	}
	if strings.HasSuffix(name, ".partial") {
		return nil, fmt.Errorf("invalid WAL filename %q: .partial suffix is not archiveable", name)
	}
	if strings.HasSuffix(name, ".backup") {
		return nil, fmt.Errorf("invalid WAL filename %q: .backup suffix is not a WAL segment", name)
	}
	if len(name) != 24 {
		return nil, fmt.Errorf("invalid WAL filename %q: expected 24 hex characters, got %d", name, len(name))
	}

	timeline, err := parseHexUint32(name[0:8])
	if err != nil {
		return nil, fmt.Errorf("invalid WAL filename %q: timeline is not valid hex: %w", name, err)
	}
	high, err := parseHexUint32(name[8:16])
	if err != nil {
		return nil, fmt.Errorf("invalid WAL filename %q: segment high is not valid hex: %w", name, err)
	}
	low, err := parseHexUint32(name[16:24])
	if err != nil {
		return nil, fmt.Errorf("invalid WAL filename %q: segment low is not valid hex: %w", name, err)
	}

	return &WALFileName{
		Timeline:     int(timeline),
		SegmentHigh:  high,
		SegmentLow:   low,
		OriginalName: name,
	}, nil
}

// StartLSN returns the inclusive start LSN for the WAL segment.
func (w *WALFileName) StartLSN() string {
	return formatLSN(w.startOffsetBytes())
}

// EndLSN returns the exclusive end LSN for the WAL segment.
func (w *WALFileName) EndLSN() string {
	return formatLSN(w.startOffsetBytes() + walSegmentSizeBytes)
}

func (w *WALFileName) startOffsetBytes() uint64 {
	return (uint64(w.SegmentHigh) << 32) + (uint64(w.SegmentLow) * walSegmentSizeBytes)
}

func formatLSN(offset uint64) string {
	upper := offset >> 32
	lower := offset & 0xFFFFFFFF
	return fmt.Sprintf("%X/%X", upper, lower)
}

func parseHexUint32(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, err
	}
	return uint32(v), nil
}
