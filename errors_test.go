// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseAPIErrorReadsParameters(t *testing.T) {
	body := strings.NewReader(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 7","parameters":{"retry_after":7,"migrate_to_chat_id":-100500}}`)
	err := parseAPIError("sendMessage", http.StatusTooManyRequests, body)

	if err.ErrorCode != 429 || err.RetryAfter != 7*time.Second {
		t.Fatalf("parsed = %+v", err)
	}
	if err.MigrateToChatID != -100500 {
		t.Fatalf("migrate_to_chat_id = %d", err.MigrateToChatID)
	}
	if !IsTooManyRequests(err) || RetryAfter(err) != 7*time.Second {
		t.Fatal("classifiers must recognize a 429")
	}
	if !strings.Contains(err.Error(), "retry after") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestParseAPIErrorSurvivesNonJSON(t *testing.T) {
	err := parseAPIError("getMe", http.StatusBadGateway, strings.NewReader("<html>502</html>"))
	if err.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d", err.StatusCode)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("message = %q, want the body kept as the description", err.Error())
	}
}

func TestIsMethodNotFoundMatchesAnOldServer(t *testing.T) {
	// This is verbatim what a Bot API 7.11 server answers for a 10.3 method.
	err := parseAPIError("sendRichMessage", http.StatusNotFound, strings.NewReader(`{"ok":false,"error_code":404,"description":"Not Found: method not found"}`))
	if !IsMethodNotFound(err) {
		t.Fatal("a missing method must be recognizable")
	}
	other := parseAPIError("sendMessage", http.StatusBadRequest, strings.NewReader(`{"ok":false,"error_code":400,"description":"Bad Request: message text is empty"}`))
	if IsMethodNotFound(other) {
		t.Fatal("a parameter error is not a missing method")
	}
}

func TestIsMessageNotModified(t *testing.T) {
	err := parseAPIError("editMessageText", http.StatusBadRequest, strings.NewReader(`{"ok":false,"description":"Bad Request: message is not modified"}`))
	if !IsMessageNotModified(err) {
		t.Fatal("the benign edit answer must be recognized")
	}
}

func TestUnreachableDestination(t *testing.T) {
	blocked := parseAPIError("sendMessage", http.StatusForbidden, strings.NewReader(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
	if !blocked.IsUnreachableDestination() {
		t.Fatal("a blocked chat is permanently unreachable")
	}
	busy := parseAPIError("sendMessage", http.StatusTooManyRequests, strings.NewReader(`{"ok":false,"description":"Too Many Requests"}`))
	if busy.IsUnreachableDestination() {
		t.Fatal("a rate limit is not a dead chat")
	}
}

// The token is in every request URL, and net/url errors quote that URL. It
// must never reach a log line.
func TestTransportErrorNeverCarriesTheToken(t *testing.T) {
	c := New(testToken, WithAPIBase("http://127.0.0.1:1"))
	c.sleep = func(context.Context, time.Duration) error { return nil }
	_, err := c.GetMe(context.Background())
	if err == nil {
		t.Fatal("want a connection error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked into %q", err.Error())
	}
	if !strings.Contains(err.Error(), "telegram transport failed") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestRedactAlsoCleansPaths(t *testing.T) {
	c := New(testToken)
	got := c.redact("/var/lib/telegram-bot-api/" + testToken + "/video_notes/file_0.mp4")
	if strings.Contains(got, testToken) {
		t.Fatalf("token leaked into a path: %q", got)
	}
}

// Telegram does not answer 200 with ok=false, but a proxy in front of it does,
// and callers classify failures by type -- so it has to arrive as an APIError
// like every other refusal.
func TestOKFalseOnA200IsAnAPIError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: can't parse entities","parameters":{"retry_after":3}}`))
	})

	_, err := c.SendMessage(context.Background(), 1, "<b>", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T (%v), want *APIError", err, err)
	}
	if apiErr.ErrorCode != 400 {
		t.Fatalf("ErrorCode = %d", apiErr.ErrorCode)
	}
	if apiErr.RetryAfter != 3*time.Second {
		t.Fatalf("RetryAfter = %s", apiErr.RetryAfter)
	}
	if len(apiErr.Response) == 0 {
		t.Fatal("the body must be kept for callers that record it")
	}
	if !strings.Contains(apiErr.Error(), "can't parse entities") {
		t.Fatalf("message = %q", apiErr.Error())
	}
}

// A 200 with ok=false and error_code 429 must still be retried: the HTTP status
// says nothing, the payload says everything.
func TestOKFalseIsClassifiedByTelegramsCode(t *testing.T) {
	err := apiErrorFrom("sendMessage", http.StatusOK,
		[]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests"}`))
	if !IsTooManyRequests(err) {
		t.Fatal("a payload-level 429 must be recognized")
	}
	if !retryableError(err) {
		t.Fatal("a payload-level 429 must be retryable")
	}
	missing := apiErrorFrom("sendRichMessage", http.StatusOK,
		[]byte(`{"ok":false,"error_code":404,"description":"Not Found: method not found"}`))
	if !IsMethodNotFound(missing) {
		t.Fatal("a payload-level 404 must still read as a missing method")
	}
}

// A well-formed success must not be mistaken for a refusal.
func TestOKTrueIsNotARefusal(t *testing.T) {
	if err := refusal("getMe", 200, []byte(`{"ok":true,"result":{"id":1}}`)); err != nil {
		t.Fatalf("refusal on a success = %v", err)
	}
	if err := refusal("getFile", 200, []byte(`not json at all`)); err != nil {
		t.Fatal("a body that is not JSON must be left to the caller's own decode")
	}
}

// Telegram never echoes the request, but a proxy answering in its place quotes
// the request URI -- and that URI is the token. One bot writes this body to a
// database column.
func TestAPIErrorBodyIsRedacted(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<html><body>The requested URL /bot" + testToken + "/sendMessage was not found on this server.</body></html>"))
	})

	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked into the message: %q", err.Error())
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T", err)
	}
	if strings.Contains(apiErr.Description, testToken) {
		t.Fatalf("token leaked into Description: %q", apiErr.Description)
	}
	if strings.Contains(string(apiErr.Response), testToken) {
		t.Fatalf("token leaked into Response: %q", apiErr.Response)
	}
}

// A body this package cannot read will read the same way next time, so the
// retry budget must not be spent on it.
func TestUnreadableAnswerIsNotRetried(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	_, err := c.GetMe(context.Background())
	if !errors.Is(err, ErrUnexpectedResult) {
		t.Fatalf("err = %v, want ErrUnexpectedResult", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1: a parse failure does not heal by repeating", calls.Load())
	}
	if !strings.Contains(err.Error(), "getMe") {
		t.Fatalf("err = %v, want the method named", err)
	}
}

// When Telegram asks for a wait the caller cannot afford, both facts have to
// survive: what Telegram said, and that the caller stopped waiting.
func TestARefusalSurvivesAnExpiredWait(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":30}}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return context.DeadlineExceeded }

	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if !IsTooManyRequests(err) {
		t.Fatalf("err = %v, want the 429 preserved", err)
	}
	if RetryAfter(err) != 30*time.Second {
		t.Fatalf("RetryAfter = %s, want what Telegram asked for", RetryAfter(err))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("the caller's expired budget must be visible too")
	}
}
