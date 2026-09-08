// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"net/http"
	"strings"
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
