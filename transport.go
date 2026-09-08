// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes bounds how much of a response is held at once. A hundred
// updates is orders of magnitude below this; anything larger is not an answer
// this package understands.
const maxResponseBytes = 32 << 20

// maxGetAttempts bounds the retry loop for idempotent calls. Four attempts
// with the backoff in retry.go stay inside a single long-poll cycle.
const maxGetAttempts = 4

func (c *Client) get(ctx context.Context, method string, values url.Values, out any) error {
	return c.getWithTimeout(ctx, method, values, out, c.timeout)
}

// getWithTimeout performs an idempotent GET with retries: transport failures
// (DNS, connect, reset) and Telegram 429/5xx responses are retried with a short
// jittered backoff, honoring Retry-After when Telegram provides one. POSTs are
// not retried this way because a lost response would double-send.
//
// Honoring Retry-After means one call can block for as long as Telegram asks,
// three times over. A caller on a path that must stay responsive -- anything
// running on an update loop -- should pass a context with its own deadline
// rather than assume this returns quickly.
func (c *Client) getWithTimeout(ctx context.Context, method string, values url.Values, out any, timeout time.Duration) error {
	var lastErr error
	for attempt := 1; attempt <= maxGetAttempts; attempt++ {
		retryAfter, err := c.attempt(ctx, timeout, func(attemptCtx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(attemptCtx, http.MethodGet, c.endpoint(method)+"?"+values.Encode(), nil)
		}, method, out, attempt)
		if err == nil {
			return nil
		}
		lastErr = err
		if ctx.Err() != nil || attempt == maxGetAttempts || !retryableError(err) {
			return err
		}
		delay := retryAfter
		if delay <= 0 {
			delay = retryDelay(attempt)
		}
		if waitErr := c.sleep(ctx, delay); waitErr != nil {
			// Both facts matter: what Telegram answered, and that the caller
			// stopped waiting for it.
			return errors.Join(lastErr, waitErr)
		}
	}
	return lastErr
}

// post sends a JSON body. A POST is replayed only when Telegram itself asked
// for it with Retry-After: any other failure may have been delivered, and a
// second send would duplicate a message.
func (c *Client) post(ctx context.Context, method string, body any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= 2; attempt++ {
		retryAfter, err := c.attempt(ctx, c.timeout, func(attemptCtx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.endpoint(method), bytes.NewReader(raw))
			if err != nil {
				return nil, err
			}
			req.Header.Set("Content-Type", "application/json")
			return req, nil
		}, method, out, attempt)
		if err == nil {
			return nil
		}
		lastErr = err
		if retryAfter <= 0 || attempt == 2 {
			return err
		}
		if waitErr := c.sleep(ctx, retryAfter); waitErr != nil {
			return errors.Join(lastErr, waitErr)
		}
	}
	return lastErr
}

// attempt runs one request under its own deadline. The response body is fully
// consumed before the deadline is released.
func (c *Client) attempt(ctx context.Context, timeout time.Duration, build func(context.Context) (*http.Request, error), method string, out any, n int) (time.Duration, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := build(attemptCtx)
	if err != nil {
		err = c.redactError(err)
		c.emit(method, 0, 0, n, err)
		return 0, err
	}
	return c.do(method, req, out, n)
}

func (c *Client) do(method string, req *http.Request, out any, n int) (time.Duration, error) {
	started := time.Now()
	resp, err := c.http.Do(req)
	if err != nil {
		err = c.redactError(err)
		c.emit(method, 0, time.Since(started), n, err)
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := c.redactAPI(parseAPIError(method, resp.StatusCode, resp.Body))
		c.emit(method, resp.StatusCode, time.Since(started), n, apiErr)
		return apiErr.RetryAfter, apiErr
	}
	// A 2xx that carries ok=false is a refusal, not a success. Telegram does
	// not answer that way, but a proxy in front of it does, and so does a
	// self-hosted server under some failures -- and callers classify failures
	// by type, so it has to arrive as an APIError like every other refusal.
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		readErr = c.redactError(readErr)
		c.emit(method, resp.StatusCode, time.Since(started), n, readErr)
		return 0, readErr
	}
	if apiErr := c.redactAPI(refusal(method, resp.StatusCode, raw)); apiErr != nil {
		c.emit(method, resp.StatusCode, time.Since(started), n, apiErr)
		return apiErr.RetryAfter, apiErr
	}
	if out == nil {
		c.emit(method, resp.StatusCode, time.Since(started), n, nil)
		return 0, nil
	}
	if err = json.Unmarshal(raw, out); err != nil {
		// The request was accepted; only its answer is unreadable. Naming the
		// method matters: the decoder's own message mentions neither Telegram
		// nor which call produced it.
		err = fmt.Errorf("%w: %s: %s", ErrUnexpectedResult, method, c.redact(err.Error()))
	}
	c.emit(method, resp.StatusCode, time.Since(started), n, err)
	return 0, err
}

func (c *Client) emit(method string, status int, d time.Duration, attempt int, err error) {
	if c.observe == nil {
		return
	}
	c.observe(Event{
		Method:    method,
		Status:    status,
		Duration:  d,
		Attempt:   attempt,
		Retryable: err != nil && retryableError(err),
		Err:       err,
	})
}

func (c *Client) endpoint(method string) string {
	return strings.TrimRight(c.apiBase, "/") + "/bot" + c.token + "/" + method
}
