package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	clock := time.Unix(0, 0)
	l := New(60, 3) // one a second, three at once
	l.now = func() time.Time { return clock }

	for i := range 3 {
		if !l.Allow("a") {
			t.Fatalf("event %d refused inside the burst", i)
		}
	}
	if l.Allow("a") {
		t.Fatal("fourth event allowed")
	}
	if !l.Allow("b") {
		t.Fatal("keys must not share a bucket")
	}
	clock = clock.Add(time.Second)
	if !l.Allow("a") {
		t.Fatal("a token should refill after a second")
	}
	if l.Allow("a") {
		t.Fatal("only one token refills in a second")
	}
	clock = clock.Add(time.Hour)
	l.Allow("c")
	if _, ok := l.buckets["b"]; ok {
		t.Fatal("full buckets should be swept")
	}
}

func TestPerHour(t *testing.T) {
	clock := time.Unix(0, 0)
	l := NewPerHour(3, 3)
	l.now = func() time.Time { return clock }
	for range 3 {
		l.Allow("a")
	}
	allowed := 0
	for range 60 {
		clock = clock.Add(time.Minute)
		if l.Allow("a") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("allowed %d in the hour after the burst, want 3", allowed)
	}
}

func TestTake(t *testing.T) {
	clock := time.Unix(0, 0)
	l := NewPerHour(3600, 100) // one a second, a hundred at once
	l.now = func() time.Time { return clock }
	if ok, _ := l.Take("a", 80); !ok {
		t.Fatal("80 of a burst of 100 refused")
	}
	ok, wait := l.Take("a", 50)
	if ok || wait != 30*time.Second {
		t.Fatalf("50 more = %v, wait %v; want refused, 30s", ok, wait)
	}
	// The refusal spent nothing.
	if ok, _ := l.Take("a", 20); !ok {
		t.Fatal("the 20 left refused")
	}
	if ok, _ := l.Take("a", 101); ok {
		t.Fatal("more than the burst taken")
	}
	clock = clock.Add(10 * time.Second)
	if ok, _ := l.Take("a", 10); !ok {
		t.Fatal("10 refilled in 10 seconds, refused")
	}
}
