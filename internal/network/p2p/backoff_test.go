package p2p

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackoffCalculation(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 10 * time.Millisecond,
		MaxInterval:     80 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      4,
		Jitter:          false, // deterministic for calculation test
	}

	b := NewBackoff(cfg)

	// Step 1: 10ms
	d1 := b.NextDelay()
	if d1 != 10*time.Millisecond {
		t.Errorf("step 1 delay = %v, want 10ms", d1)
	}
	if b.Attempts() != 1 {
		t.Errorf("attempts = %d, want 1", b.Attempts())
	}

	// Step 2: 20ms
	d2 := b.NextDelay()
	if d2 != 20*time.Millisecond {
		t.Errorf("step 2 delay = %v, want 20ms", d2)
	}

	// Step 3: 40ms
	d3 := b.NextDelay()
	if d3 != 40*time.Millisecond {
		t.Errorf("step 3 delay = %v, want 40ms", d3)
	}

	// Step 4: 80ms (capped at MaxInterval)
	d4 := b.NextDelay()
	if d4 != 80*time.Millisecond {
		t.Errorf("step 4 delay = %v, want 80ms", d4)
	}

	// Step 5: capped at MaxInterval
	d5 := b.NextDelay()
	if d5 != 80*time.Millisecond {
		t.Errorf("step 5 delay = %v, want 80ms", d5)
	}

	// Reset
	b.Reset()
	if b.Attempts() != 0 {
		t.Errorf("after reset attempts = %d, want 0", b.Attempts())
	}
	dReset := b.NextDelay()
	if dReset != 10*time.Millisecond {
		t.Errorf("after reset delay = %v, want 10ms", dReset)
	}
}

func TestBackoffLimits(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 5 * time.Millisecond,
		MaxInterval:     20 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      3,
		Jitter:          false,
	}

	b := NewBackoff(cfg)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if !b.HasNext() {
			t.Fatalf("expected HasNext to be true on attempt %d", i+1)
		}
		if err := b.Sleep(ctx); err != nil {
			t.Fatalf("unexpected sleep error: %v", err)
		}
	}

	if b.HasNext() {
		t.Errorf("expected HasNext to be false after %d attempts", cfg.MaxRetries)
	}

	err := b.Sleep(ctx)
	if !errors.Is(err, ErrMaxRetriesReached) {
		t.Errorf("expected ErrMaxRetriesReached, got %v", err)
	}
}

func TestRetrySuccess(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 1 * time.Millisecond,
		MaxInterval:     5 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
		Jitter:          false,
	}

	attempts := 0
	err := Retry(context.Background(), cfg, func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Retry unexpected error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	cfg := BackoffConfig{
		InitialInterval: 50 * time.Millisecond,
		MaxInterval:     200 * time.Millisecond,
		Multiplier:      2.0,
		MaxRetries:      5,
		Jitter:          false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := Retry(ctx, cfg, func(ctx context.Context) error {
		return errors.New("persistently failing")
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}
