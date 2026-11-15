package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"
)

const defaultFailRate = 0.5

// ErrSimulatedFailure is what Flaky returns when it fails on purpose.
var ErrSimulatedFailure = errors.New("flaky: simulated failure")

type flakyParams struct {
	// FailRate is a pointer so we can tell "not given" (nil) apart from
	// an explicit 0.
	FailRate *float64 `json:"fail_rate"`
	Duration string   `json:"duration"`
}

func parseFlaky(payload json.RawMessage) (failRate float64, d time.Duration, err error) {
	var p flakyParams
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0, 0, fmt.Errorf("flaky payload: %w", err)
	}

	failRate = defaultFailRate
	if p.FailRate != nil {
		failRate = *p.FailRate
	}
	if failRate < 0 || failRate > 1 {
		return 0, 0, errors.New("fail_rate must be between 0 and 1")
	}

	if p.Duration != "" {
		d, err = time.ParseDuration(p.Duration)
		if err != nil {
			return 0, 0, fmt.Errorf("duration %q: use a value like \"500ms\" or \"2s\"", p.Duration)
		}
		if d < 0 {
			return 0, 0, errors.New("duration must not be negative")
		}
	}

	return failRate, d, nil
}

// ValidateFlaky checks "fail_rate" (0 to 1, default 0.5) and "duration"
// (like "2s", default 0).
func ValidateFlaky(payload json.RawMessage) error {
	_, _, err := parseFlaky(payload)
	return err
}

// Flaky works for "duration", then fails with probability "fail_rate".
// It exists to demonstrate retries, timeouts and shutdown.
func Flaky(ctx context.Context, payload json.RawMessage) error {
	failRate, d, err := parseFlaky(payload)
	if err != nil {
		return err
	}

	if err := sleep(ctx, d); err != nil {
		return err
	}

	// rand.Float64 returns a number in [0, 1), so a rate of 0 never fails
	// and a rate of 1 always fails.
	if rand.Float64() < failRate {
		return ErrSimulatedFailure
	}
	return nil
}
