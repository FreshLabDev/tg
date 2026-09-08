// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetRetriesOnTooManyRequests(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests: retry later","parameters":{"retry_after":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"bot"}}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	me, err := c.GetMe(context.Background())
	if err != nil {
		t.Fatalf("GetMe after a 429: %v", err)
	}
	if me.Username != "bot" {
		t.Fatalf("username = %q, want %q", me.Username, "bot")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("server calls = %d, want 2", got)
	}
}

func TestGetGivesUpOnClientError(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	})

	if _, err := c.GetMe(context.Background()); err == nil {
		t.Fatal("want an error for 401")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("server calls = %d, want 1: a 401 never becomes valid by repeating", got)
	}
}

// A POST that failed may still have been delivered, so it is replayed only
// when Telegram itself asked with retry_after.
func TestPostIsNotRetriedOnServerError(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := c.SendMessage(context.Background(), 1, "hi", nil); err == nil {
		t.Fatal("want an error for 502")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("server calls = %d, want 1: a send must not be duplicated", got)
	}
}

func TestPostRetriesOnRetryAfter(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":429,"description":"Too Many Requests","parameters":{"retry_after":2}}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":11}}`))
	})
	var waited time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error { waited = d; return nil }

	msg, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if msg.MessageID != 11 {
		t.Fatalf("message_id = %d, want 11", msg.MessageID)
	}
	if waited != 2*time.Second {
		t.Fatalf("waited %s, want the 2s Telegram asked for", waited)
	}
}

func TestObserverSeesEveryAttempt(t *testing.T) {
	var calls atomic.Int32
	var events []Event
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"ok":false,"description":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7}}`))
	}, WithObserver(func(e Event) { events = append(events, e) }))
	c.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := c.GetMe(context.Background()); err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("observed %d attempts, want 2", len(events))
	}
	if events[0].Status != 500 || events[0].Attempt != 1 || !events[0].Retryable {
		t.Fatalf("first event = %+v, want a retryable 500 on attempt 1", events[0])
	}
	if events[1].Status != 200 || events[1].Err != nil || events[1].Method != "getMe" {
		t.Fatalf("second event = %+v, want a clean getMe", events[1])
	}
	if events[1].Duration <= 0 {
		t.Fatal("duration must be measured")
	}
}

func TestGetUpdatesSendsAllowedUpdates(t *testing.T) {
	var got string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query().Get("allowed_updates")
		_, _ = w.Write([]byte(`{"ok":true,"result":[]}`))
	}, WithAllowedUpdates("message", "callback_query"))

	if _, err := c.GetUpdates(context.Background(), 0, 1); err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if got != `["message","callback_query"]` {
		t.Fatalf("allowed_updates = %q", got)
	}
}

func TestSetMyCommandsLiftsLanguageCode(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	err := c.SetMyCommandsForScope(context.Background(), []BotCommand{{Command: "start", Description: "s"}},
		&BotCommandScope{Type: "default", LanguageCode: "uk"})
	if err != nil {
		t.Fatalf("SetMyCommandsForScope: %v", err)
	}
	if body["language_code"] != "uk" {
		t.Fatalf("language_code = %v, want it beside scope, not inside it", body["language_code"])
	}
	scope, _ := body["scope"].(map[string]any)
	if _, inside := scope["language_code"]; inside {
		t.Fatal("language_code must not be serialized inside scope")
	}
}

func TestSendMessageIsHTMLWithoutLinkPreview(t *testing.T) {
	var body map[string]any
	var path string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})

	if _, err := c.SendMessage(context.Background(), 42, "<b>hi</b>", nil); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if !strings.HasSuffix(path, "/sendMessage") {
		t.Fatalf("path = %q", path)
	}
	if body["parse_mode"] != "HTML" {
		t.Fatalf("parse_mode = %v", body["parse_mode"])
	}
	preview, _ := body["link_preview_options"].(map[string]any)
	if preview["is_disabled"] != true {
		t.Fatalf("link_preview_options = %v", body["link_preview_options"])
	}
}

// A bot that keeps an audit trail stores what Telegram actually said, and a
// bot needing a field this package does not model reads it out of Raw.
func TestMessageKeepsItsRawJSON(t *testing.T) {
	const result = `{"message_id":7,"date":1700000000,"chat":{"id":42,"type":"private"},"text":"hi","some_future_field":{"a":1}}`
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":` + result + `}`))
	})

	msg, err := c.SendMessage(context.Background(), 42, "hi", nil)
	if err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	if string(msg.Raw) != result {
		t.Fatalf("Raw = %s\nwant %s", msg.Raw, result)
	}
	var reread map[string]any
	if err := json.Unmarshal(msg.Raw, &reread); err != nil {
		t.Fatalf("Raw must stay valid JSON: %v", err)
	}
	if _, ok := reread["some_future_field"]; !ok {
		t.Fatal("Raw must carry fields this package does not model")
	}
	// A nested message keeps its own bytes too.
	var upd Update
	if err := json.Unmarshal([]byte(`{"update_id":1,"message":{"message_id":2,"reply_to_message":{"message_id":1,"text":"orig"}}}`), &upd); err != nil {
		t.Fatal(err)
	}
	if upd.Message.ReplyToMessage == nil || len(upd.Message.ReplyToMessage.Raw) == 0 {
		t.Fatal("a nested message must keep its raw bytes as well")
	}
}

func TestAPIErrorKeepsTheRawBody(t *testing.T) {
	const body = `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	})

	_, err := c.SendMessage(context.Background(), 1, "x", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T", err)
	}
	if string(apiErr.Response) != body {
		t.Fatalf("Response = %s, want the body verbatim", apiErr.Response)
	}
}

// The fallback send must not carry a parse mode: the point of it is that
// nothing about the text can make Telegram refuse the message.
func TestSendPlainTextHasNoParseMode(t *testing.T) {
	var body map[string]any
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	})

	if _, err := c.SendPlainText(context.Background(), 42, "<b>not markup</b>"); err != nil {
		t.Fatalf("SendPlainText: %v", err)
	}
	if _, present := body["parse_mode"]; present {
		t.Fatalf("parse_mode = %v, want it absent", body["parse_mode"])
	}
	if body["text"] != "<b>not markup</b>" {
		t.Fatalf("text = %v, want it sent verbatim", body["text"])
	}
	preview, _ := body["link_preview_options"].(map[string]any)
	if preview["is_disabled"] != true {
		t.Fatal("previews stay off for the fallback too")
	}
}

// Telegram accepted the request and acted on it; only the receipt is
// unreadable. A caller that must not repeat itself needs to tell that apart
// from a refusal.
func TestUnreadableResultIsNotARefusal(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})

	_, err := c.SendMessage(context.Background(), 1, "delivered", nil)
	if err == nil {
		t.Fatal("want an error: the caller asked for a message and got none")
	}
	if !errors.Is(err, ErrUnexpectedResult) {
		t.Fatalf("err = %v, want it to wrap ErrUnexpectedResult", err)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		t.Fatal("an unreadable result is not an API refusal")
	}
	if !strings.Contains(err.Error(), "sendMessage") {
		t.Fatalf("err = %v, want the method named", err)
	}
}

// The three methods that answer with a bare true were not checking ok at all.
func TestOKFalseIsCaughtForMethodsWithNoResult(t *testing.T) {
	for _, method := range []string{"answerCallbackQuery", "setMyCommands", "editMessageText"} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: refused"}`))
		})
		var err error
		switch method {
		case "answerCallbackQuery":
			err = c.AnswerCallbackQuery(context.Background(), "cb", "")
		case "setMyCommands":
			err = c.SetMyCommands(context.Background(), []BotCommand{{Command: "start"}})
		case "editMessageText":
			err = c.EditMessageText(context.Background(), 1, 2, "text", nil)
		}
		var apiErr *APIError
		if !errors.As(err, &apiErr) {
			t.Fatalf("%s: err = %v, want an *APIError", method, err)
		}
	}
}
