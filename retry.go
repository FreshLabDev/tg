// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

// retryableError reports whether an idempotent request is worth retrying:
// Telegram 429/5xx responses and transport-level failures qualify; a canceled
// caller context never does (the retry loop checks the caller's ctx separately,
// so an expired per-attempt deadline still retries).
func retryableError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return !errors.Is(err, context.Canceled)
}

// retryDelay backs off 500ms, 1s, 2s (jittered): transient hiccups recover fast
// and three retries stay well inside one long-poll cycle.
func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 3 {
		attempt = 3
	}
	return jitterDuration(500 * time.Millisecond << (attempt - 1))
}

// jitterDuration spreads a delay across [d/2, 3d/2) so callers that failed
// together do not retry in a synchronized wave.
func jitterDuration(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	half := int64(d / 2)
	return d - time.Duration(half) + time.Duration(rand.Int63n(2*half+1))
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
