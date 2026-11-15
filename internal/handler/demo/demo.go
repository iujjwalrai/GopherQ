// Package demo contains example job handlers: a fake email sender, a
// handler that fails on purpose, and a real webhook caller.
package demo

import (
	"context"
	"time"
)

// sleep waits for d, or returns early with ctx's error if ctx is
// cancelled first.
func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
