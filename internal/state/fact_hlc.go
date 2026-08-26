package state

import (
	"fmt"
	"strconv"
	"strings"
)

const factEnvelopeVersion = 1

// HLC is the hybrid logical clock carried on every fact envelope.
type HLC struct {
	WallMS  int64
	Logical int64
}

func (h HLC) String() string {
	return fmt.Sprintf("%020d:%06d", h.WallMS, h.Logical)
}

func parseHLC(value string) (HLC, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return HLC{}, fmt.Errorf("invalid hlc %q", value)
	}
	wallMS, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return HLC{}, fmt.Errorf("parse hlc wall component: %w", err)
	}
	logical, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return HLC{}, fmt.Errorf("parse hlc logical component: %w", err)
	}
	return HLC{WallMS: wallMS, Logical: logical}, nil
}

func compareHLC(left, right HLC) int {
	if left.WallMS != right.WallMS {
		if left.WallMS < right.WallMS {
			return -1
		}
		return 1
	}
	if left.Logical != right.Logical {
		if left.Logical < right.Logical {
			return -1
		}
		return 1
	}
	return 0
}

func compareFactOrder(hlcLeft HLC, envLeft, idLeft string, hlcRight HLC, envRight, idRight string) int {
	if cmp := compareHLC(hlcLeft, hlcRight); cmp != 0 {
		return cmp
	}
	if cmp := strings.Compare(envLeft, envRight); cmp != 0 {
		return cmp
	}
	return strings.Compare(idLeft, idRight)
}

func advanceHLC(clock HLC, nowMS int64, seen HLC) HLC {
	next := HLC{WallMS: nowMS, Logical: 0}
	if compareHLC(clock, next) > 0 {
		next = clock
	}
	if compareHLC(seen, next) > 0 {
		next = seen
	}
	if next.WallMS == clock.WallMS && next.Logical == clock.Logical {
		next.Logical++
	}
	return next
}
