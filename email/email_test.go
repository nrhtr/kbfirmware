package email

import (
	"testing"
	"time"
)

func TestNextDigestTime_BeforeTarget(t *testing.T) {
	loc, _ := time.LoadLocation("Australia/Melbourne")

	// 10:00am Melbourne — next digest should be 6:30pm today
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, loc)
	next := nextDigestTimeFrom(now, loc)

	want := time.Date(2025, 6, 15, 18, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Errorf("before target: got %v want %v", next, want)
	}
}

func TestNextDigestTime_AfterTarget(t *testing.T) {
	loc, _ := time.LoadLocation("Australia/Melbourne")

	// 9:00pm Melbourne — next digest should be 6:30pm tomorrow
	now := time.Date(2025, 6, 15, 21, 0, 0, 0, loc)
	next := nextDigestTimeFrom(now, loc)

	want := time.Date(2025, 6, 16, 18, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Errorf("after target: got %v want %v", next, want)
	}
}

func TestNextDigestTime_ExactlyAtTarget(t *testing.T) {
	loc, _ := time.LoadLocation("Australia/Melbourne")

	// Exactly 6:30pm — should schedule for tomorrow (not ≤ target)
	now := time.Date(2025, 6, 15, 18, 30, 0, 0, loc)
	next := nextDigestTimeFrom(now, loc)

	want := time.Date(2025, 6, 16, 18, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Errorf("at target: got %v want %v", next, want)
	}
}
