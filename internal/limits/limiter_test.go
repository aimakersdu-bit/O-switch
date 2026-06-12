package limits

import "testing"

func TestLimiterTryAcquireAndRelease(t *testing.T) {
	limiter := NewLimiter(1)
	if !limiter.TryAcquire() {
		t.Fatalf("first acquire should succeed")
	}
	if limiter.TryAcquire() {
		t.Fatalf("second acquire should fail")
	}
	if limiter.Available() != 0 {
		t.Fatalf("available should be 0")
	}
	limiter.Release()
	if limiter.Available() != 1 {
		t.Fatalf("available should be 1 after release")
	}
	if !limiter.TryAcquire() {
		t.Fatalf("acquire should succeed after release")
	}
}
