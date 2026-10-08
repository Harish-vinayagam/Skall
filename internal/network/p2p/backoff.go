package p2p

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// ErrMaxRetriesReached is returned when an operation fails and the retry cap is reached.
var ErrMaxRetriesReached = errors.New("maximum retry attempts reached")

// BackoffConfig configures the exponential backoff parameters.
type BackoffConfig struct {
	// InitialInterval is the duration of the first backoff delay. Defaults to 1 second.
	InitialInterval time.Duration

	// MaxInterval is the upper bound on the backoff delay. Defaults to 60 seconds.
	MaxInterval time.Duration

	// Multiplier is the factor by which the interval is multiplied on each step. Defaults to 2.0.
	Multiplier float64

	// MaxRetries is the maximum number of retry attempts before giving up (0 = unlimited).
	MaxRetries int

	// Jitter enables randomized jitter (±20%) to avoid synchronized retry storms.
	Jitter bool
}

// DefaultBackoffConfig returns a production-ready BackoffConfig.
func DefaultBackoffConfig() BackoffConfig {
	return BackoffConfig{
		InitialInterval: 1 * time.Second,
		MaxInterval:     60 * time.Second,
		Multiplier:      2.0,
		MaxRetries:      5,
		Jitter:          true,
	}
}

// Backoff manages step-by-step exponential backoff calculations.
type Backoff struct {
	cfg          BackoffConfig
	currentDelay time.Duration
	attempts     int
	rng          *rand.Rand
}

// NewBackoff constructs a new Backoff calculator from cfg.
func NewBackoff(cfg BackoffConfig) *Backoff {
	if cfg.InitialInterval <= 0 {
		cfg.InitialInterval = 1 * time.Second
	}
	if cfg.MaxInterval <= 0 {
		cfg.MaxInterval = 60 * time.Second
	}
	if cfg.MaxInterval < cfg.InitialInterval {
		cfg.MaxInterval = cfg.InitialInterval
	}
	if cfg.Multiplier <= 1.0 {
		cfg.Multiplier = 2.0
	}

	return &Backoff{
		cfg:          cfg,
		currentDelay: cfg.InitialInterval,
		attempts:     0,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Reset resets the backoff state to its initial delay and 0 attempts.
func (b *Backoff) Reset() {
	b.currentDelay = b.cfg.InitialInterval
	b.attempts = 0
}

// Attempts returns the number of retries attempted so far.
func (b *Backoff) Attempts() int {
	return b.attempts
}

// HasNext returns true if another retry attempt is permitted under MaxRetries.
func (b *Backoff) HasNext() bool {
	if b.cfg.MaxRetries <= 0 {
		return true // unlimited
	}
	return b.attempts < b.cfg.MaxRetries
}

// NextDelay calculates the next sleep duration and advances the backoff counter.
func (b *Backoff) NextDelay() time.Duration {
	delay := b.currentDelay
	b.attempts++

	// Advance currentDelay for the next iteration
	next := time.Duration(float64(b.currentDelay) * b.cfg.Multiplier)
	if next > b.cfg.MaxInterval {
		next = b.cfg.MaxInterval
	}
	b.currentDelay = next

	if b.cfg.Jitter {
		// Apply ±20% jitter: range [0.8 * delay, 1.2 * delay]
		factor := 0.8 + (b.rng.Float64() * 0.4)
		delay = time.Duration(float64(delay) * factor)
	}

	return delay
}

// Sleep pauses for the next delay duration, returning early if ctx is cancelled.
func (b *Backoff) Sleep(ctx context.Context) error {
	if !b.HasNext() {
		return ErrMaxRetriesReached
	}
	delay := b.NextDelay()
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Retry executes op repeatedly with exponential backoff until it succeeds,
// ctx is cancelled, or the retry limit is reached.
func Retry(ctx context.Context, cfg BackoffConfig, op func(ctx context.Context) error) error {
	b := NewBackoff(cfg)
	var lastErr error

	for {
		if err := op(ctx); err == nil {
			return nil
		} else {
			lastErr = err
		}

		if err := b.Sleep(ctx); err != nil {
			if errors.Is(err, ErrMaxRetriesReached) {
				return lastErr
			}
			return err
		}
	}
}
