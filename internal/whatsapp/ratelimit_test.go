package whatsapp

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestRateLimiterWindowStartBucketsByWindow(t *testing.T) {
	l := NewRateLimiter(nil, nil)

	first := l.windowStart(time.Date(2026, 9, 27, 10, 15, 3, 0, time.UTC))
	sameWindow := l.windowStart(time.Date(2026, 9, 27, 10, 15, 57, 0, time.UTC))
	nextWindow := l.windowStart(time.Date(2026, 9, 27, 10, 16, 0, 0, time.UTC))

	if !first.Equal(sameWindow) {
		t.Fatalf("two sends inside one window landed in different windows: %s vs %s", first, sameWindow)
	}
	if first.Equal(nextWindow) {
		t.Fatalf("a new window must get its own bucket: %s", first)
	}
	if first.Location() != time.UTC {
		t.Fatal("window starts must be stored in UTC so every instance agrees")
	}
}

func TestRateLimiterWithoutPoolIsOpen(t *testing.T) {
	l := NewRateLimiter(nil, nil)
	for i := 0; i < 3; i++ {
		if err := l.Allow(t.Context(), uuid.New()); err != nil {
			t.Fatalf("a limiter with no pool must let sends through: %v", err)
		}
	}
}
