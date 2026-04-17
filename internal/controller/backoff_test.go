package controller

import (
	"testing"
	"time"
)

func TestBackoffDelay(t *testing.T) {
	base := 5 * time.Minute

	tests := []struct {
		name     string
		failures int
		want     time.Duration
	}{
		{"first failure", 1, 5 * time.Minute},
		{"second failure", 2, 10 * time.Minute},
		{"third failure capped", 3, 10 * time.Minute},
		{"many failures capped", 10, 10 * time.Minute},
		{"zero failures", 0, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := backoffDelay(base, tt.failures)
			if got != tt.want {
				t.Errorf("backoffDelay(%v, %d) = %v, want %v", base, tt.failures, got, tt.want)
			}
		})
	}
}

func TestErrorTracker_RecordAndReset(t *testing.T) {
	tracker := newErrorTracker()
	base := 1 * time.Minute

	// First error: 1min
	d1 := tracker.recordError("test/app", base)
	if d1 != 1*time.Minute {
		t.Errorf("first error: got %v, want 1m", d1)
	}

	// Second error: 2min
	d2 := tracker.recordError("test/app", base)
	if d2 != 2*time.Minute {
		t.Errorf("second error: got %v, want 2m", d2)
	}

	// Third error: 4min
	d3 := tracker.recordError("test/app", base)
	if d3 != 4*time.Minute {
		t.Errorf("third error: got %v, want 4m", d3)
	}

	// Reset on success
	tracker.recordSuccess("test/app")

	// Next error starts over: 1min
	d4 := tracker.recordError("test/app", base)
	if d4 != 1*time.Minute {
		t.Errorf("after reset: got %v, want 1m", d4)
	}
}

func TestErrorTracker_IndependentKeys(t *testing.T) {
	tracker := newErrorTracker()
	base := 1 * time.Minute

	tracker.recordError("ns/app-a", base)
	tracker.recordError("ns/app-a", base) // 2nd error for app-a

	d := tracker.recordError("ns/app-b", base) // 1st error for app-b
	if d != 1*time.Minute {
		t.Errorf("app-b first error should be 1m, got %v", d)
	}
}
