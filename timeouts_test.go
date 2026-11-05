// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// deadlines records how long each request had left before its deadline when
// it went out. A timeout is the deadline a call puts on its request, so the
// tests read the deadline instead of racing a slow server against it.
type deadlines struct {
	next http.RoundTripper
	mu   sync.Mutex
	left map[string]time.Duration
}

func (d *deadlines) RoundTrip(req *http.Request) (*http.Response, error) {
	if dl, ok := req.Context().Deadline(); ok {
		d.mu.Lock()
		d.left[path.Base(req.URL.Path)] = time.Until(dl)
		d.mu.Unlock()
	}
	return d.next.RoundTrip(req)
}

func (d *deadlines) of(method string) time.Duration {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.left[method]
}

func deadlineClient(t *testing.T, opts ...Option) (*Client, *deadlines) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMediaGroup") {
			_, _ = w.Write([]byte(`{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	t.Cleanup(srv.Close)
	d := &deadlines{next: srv.Client().Transport, left: map[string]time.Duration{}}
	opts = append([]Option{WithAPIBase(srv.URL), WithHTTPClient(&http.Client{Transport: d})}, opts...)
	return New(testToken, opts...), d
}

// A call that carries a file gets the upload timeout; the same client's text
// send, and a send of a file Telegram already has, get the ordinary one.
func TestUploadTimeoutCoversOnlyCallsThatCarryAFile(t *testing.T) {
	c, d := deadlineClient(t, WithTimeout(time.Minute), WithUploadTimeout(time.Hour))
	ctx := context.Background()
	long := func(method string) {
		t.Helper()
		if left := d.of(method); left <= time.Minute {
			t.Fatalf("%s had %s left, want the upload timeout", method, left)
		}
	}
	short := func(method string) {
		t.Helper()
		if left := d.of(method); left <= 0 || left > time.Minute {
			t.Fatalf("%s had %s left, want the ordinary timeout", method, left)
		}
	}

	if _, err := c.SendVideo(ctx, 1, InputFileLocal{Path: "/data/big.mp4"}, VideoOptions{}); err != nil {
		t.Fatalf("SendVideo by path: %v", err)
	}
	long("sendVideo")
	if _, err := c.SendDocument(ctx, 1, InputFileUpload{Filename: "a.bin", Data: strings.NewReader("x")}, DocumentOptions{}); err != nil {
		t.Fatalf("SendDocument upload: %v", err)
	}
	long("sendDocument")
	if _, err := c.SendMediaGroup(ctx, 1, []InputMedia{
		InputMediaVideo{Media: InputFileLocal{Path: "/data/a.mp4"}},
		InputMediaVideo{Media: InputFileString{Data: "known"}},
	}, MediaGroupOptions{}); err != nil {
		t.Fatalf("SendMediaGroup with a path: %v", err)
	}
	long("sendMediaGroup")
	if err := c.CallMultipart(ctx, "sendVoice", map[string]any{"chat_id": 1}, map[string]InputFile{"voice": InputFileLocal{Path: "/data/v.ogg"}}, nil); err != nil {
		t.Fatalf("CallMultipart with a path: %v", err)
	}
	long("sendVoice")

	if _, err := c.SendMessage(ctx, 1, "hi", nil); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	short("sendMessage")
	if _, err := c.SendVideo(ctx, 1, InputFileString{Data: "known"}, VideoOptions{}); err != nil {
		t.Fatalf("SendVideo by file_id: %v", err)
	}
	short("sendVideo")
}

// Without the option a file waits no longer than a text.
func TestWithoutUploadTimeoutAFileHasTheOrdinaryTimeout(t *testing.T) {
	c, d := deadlineClient(t, WithTimeout(time.Minute))
	if _, err := c.SendVideo(context.Background(), 1, InputFileLocal{Path: "/data/big.mp4"}, VideoOptions{}); err != nil {
		t.Fatal(err)
	}
	if left := d.of("sendVideo"); left <= 0 || left > time.Minute {
		t.Fatalf("sendVideo had %s left, want the ordinary timeout", left)
	}
}

// The deadline is enforced: a server slower than the timeout fails the call.
func TestTheTimeoutCutsASlowAnswer(t *testing.T) {
	release := make(chan struct{})
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
	}, WithTimeout(5*time.Millisecond))
	// Registered after the server's own cleanup, so it runs first and lets
	// the handler return before the server waits for it.
	t.Cleanup(func() { close(release) })
	if _, err := c.SendMessage(context.Background(), 1, "hi", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the deadline", err)
	}
}

// A retry_after past the ceiling comes back at once: a send does not sleep an
// hour, and the caller learns how long Telegram asked for.
func TestRetryAfterPastTheCeilingComesBackAtOnce(t *testing.T) {
	var calls atomic.Int32
	flood := func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 3600","parameters":{"retry_after":3600}}`))
	}
	c, _ := newTestClient(t, flood)
	c.sleep = func(context.Context, time.Duration) error {
		t.Error("slept on a retry_after past the ceiling")
		return nil
	}

	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if !IsTooManyRequests(err) || RetryAfter(err) != time.Hour {
		t.Fatalf("err = %v, want the 429 with its retry_after", err)
	}
	if _, err := c.GetMe(context.Background()); !IsTooManyRequests(err) {
		t.Fatalf("GetMe: err = %v, want the 429", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("server calls = %d, want one per call", got)
	}
}

// The ceiling is the client's to move, and a wait within it is still made.
func TestRetryAfterWithinTheCeilingIsWaited(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":90}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":2}}`))
	}, WithMaxRetryAfter(2*time.Minute))
	var waited time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waited = d; return nil }

	if _, err := c.SendMessage(context.Background(), 1, "hi", nil); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if waited != 90*time.Second || calls.Load() != 2 {
		t.Fatalf("waited %s over %d calls, want 90s and a replay", waited, calls.Load())
	}
}
