// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// capture returns a stub that records the method and decoded body of one call.
func capture(t *testing.T, result string) (http.HandlerFunc, *string, *map[string]any) {
	t.Helper()
	method := new(string)
	body := new(map[string]any)
	return func(w http.ResponseWriter, r *http.Request) {
		*method = r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, body)
		_, _ = w.Write([]byte(result))
	}, method, body
}

func TestSendRichHTMLUsesTheHTMLFieldWithoutEntityDetection(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":5}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendRichHTML(context.Background(), 42, 7, 3, "<b>text</b>", nil)
	if err != nil {
		t.Fatalf("SendRichHTML: %v", err)
	}
	if msg.MessageID != 5 {
		t.Fatalf("message_id = %d", msg.MessageID)
	}
	if *method != "sendRichMessage" {
		t.Fatalf("method = %q", *method)
	}
	rich, _ := (*body)["rich_message"].(map[string]any)
	if rich["html"] != "<b>text</b>" {
		t.Fatalf("rich_message = %v, want the payload in html", rich)
	}
	if _, hasMarkdown := rich["markdown"]; hasMarkdown {
		t.Fatal("markdown would additionally parse *, _, # and backticks inside the payload")
	}
	if rich["skip_entity_detection"] != true {
		t.Fatal("entity detection must stay off so digits and hashtags are not linkified")
	}
	reply, _ := (*body)["reply_parameters"].(map[string]any)
	if reply["allow_sending_without_reply"] != true {
		t.Fatal("a deleted original must not fail the send")
	}
	if (*body)["message_thread_id"] != float64(3) {
		t.Fatalf("message_thread_id = %v", (*body)["message_thread_id"])
	}
}

func TestSendRichHTMLOmitsReplyWhenThereIsNone(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":5}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendRichHTML(context.Background(), 42, 0, 0, "text", nil); err != nil {
		t.Fatalf("SendRichHTML: %v", err)
	}
	if _, has := (*body)["reply_parameters"]; has {
		t.Fatal("reply_parameters must be absent when replyTo is zero")
	}
	if _, has := (*body)["message_thread_id"]; has {
		t.Fatal("message_thread_id must be absent outside a topic")
	}
}

func TestEditMessageRichHTMLTreatsNotModifiedAsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: message is not modified"}`))
	})

	if err := c.EditMessageRichHTML(context.Background(), 1, 2, "same", nil); err != nil {
		t.Fatalf("an unchanged edit must not be an error: %v", err)
	}
}

func TestEditMessageRichHTMLSendsHTMLAndNeverMarkdown(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.EditMessageRichHTML(context.Background(), 42, 7, "<b>done</b>", nil); err != nil {
		t.Fatalf("EditMessageRichHTML: %v", err)
	}
	if *method != "editMessageText" {
		t.Fatalf("method = %q", *method)
	}
	want := map[string]any{"html": "<b>done</b>", "skip_entity_detection": true}
	if !reflect.DeepEqual((*body)["rich_message"], want) {
		t.Fatalf("rich_message = %v, want %v", (*body)["rich_message"], want)
	}
}

func TestEditMessageRichMarkdownSendsTheRichPayload(t *testing.T) {
	const terms = "# Terms\n\nWrite to @operator or see https://example.test/terms"
	panel := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Accept", CallbackData: "agree:1", Style: StyleSuccess},
	}}}
	tests := []struct {
		name         string
		opts         []RichOption
		markup       *InlineKeyboardMarkup
		wantRich     map[string]any
		wantKeyboard bool
	}{
		{
			name:         "entity detection is off unless asked",
			markup:       panel,
			wantRich:     map[string]any{"markdown": terms, "skip_entity_detection": true},
			wantKeyboard: true,
		},
		{
			name:         "entity detection on",
			opts:         []RichOption{WithEntityDetection()},
			markup:       panel,
			wantRich:     map[string]any{"markdown": terms},
			wantKeyboard: true,
		},
		{
			name:     "no markup leaves the keyboard alone",
			opts:     []RichOption{WithEntityDetection()},
			wantRich: map[string]any{"markdown": terms},
		},
		{
			name:     "a nil option is ignored",
			opts:     []RichOption{nil},
			wantRich: map[string]any{"markdown": terms, "skip_entity_detection": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, method, body := capture(t, `{"ok":true,"result":true}`)
			c, _ := newTestClient(t, handler)

			if err := c.EditMessageRichMarkdown(context.Background(), -1001234567890, 77, terms, tt.markup, tt.opts...); err != nil {
				t.Fatalf("EditMessageRichMarkdown: %v", err)
			}
			if *method != "editMessageText" {
				t.Fatalf("method = %q, want editMessageText: a rich edit is an ordinary edit with rich_message", *method)
			}
			if (*body)["chat_id"] != float64(-1001234567890) || (*body)["message_id"] != float64(77) {
				t.Fatalf("chat_id = %v, message_id = %v", (*body)["chat_id"], (*body)["message_id"])
			}
			if !reflect.DeepEqual((*body)["rich_message"], tt.wantRich) {
				t.Fatalf("rich_message = %v, want %v", (*body)["rich_message"], tt.wantRich)
			}
			for _, field := range []string{"text", "parse_mode"} {
				if _, has := (*body)[field]; has {
					t.Fatalf("%s must not accompany rich_message: Telegram wants one or the other", field)
				}
			}
			markup, hasMarkup := (*body)["reply_markup"].(map[string]any)
			if hasMarkup != tt.wantKeyboard {
				t.Fatalf("reply_markup = %v, want present = %v", (*body)["reply_markup"], tt.wantKeyboard)
			}
			if tt.wantKeyboard {
				rows, _ := markup["inline_keyboard"].([]any)
				if len(rows) != 1 {
					t.Fatalf("inline_keyboard = %v", markup["inline_keyboard"])
				}
				button, _ := rows[0].([]any)[0].(map[string]any)
				if button["callback_data"] != "agree:1" || button["style"] != StyleSuccess {
					t.Fatalf("button = %v", button)
				}
			}
		})
	}
}

// Tapping the same button twice re-renders identical content, and Telegram
// answers that with a 400. It is the common case, not a failure.
func TestEditMessageRichMarkdownTreatsNotModifiedAsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"}`))
	})

	if err := c.EditMessageRichMarkdown(context.Background(), 1, 2, "same", nil, WithEntityDetection()); err != nil {
		t.Fatalf("an unchanged edit must not be an error: %v", err)
	}
}

func TestEditMessageRichMarkdownReturnsEveryOtherFailure(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message to edit not found"}`))
	})

	err := c.EditMessageRichMarkdown(context.Background(), 1, 2, "text", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("error = %v, want the 400 APIError", err)
	}
	if IsMessageNotModified(err) {
		t.Fatal("a missing message is not 'not modified'")
	}
}

// makeitMD renders user-authored Markdown, where a bare URL is expected to
// become a link. Everything else in the family sends generated text, where it
// is not.
func TestEntityDetectionIsOffUnlessAsked(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendRichMarkdown(context.Background(), 1, "see https://example.test", nil); err != nil {
		t.Fatalf("SendRichMarkdown: %v", err)
	}
	rich, _ := (*body)["rich_message"].(map[string]any)
	if rich["skip_entity_detection"] != true {
		t.Fatalf("rich_message = %v, want detection off by default", rich)
	}

	handler2, _, body2 := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c2, _ := newTestClient(t, handler2)
	if _, err := c2.SendRichMarkdown(context.Background(), 1, "see https://example.test", nil, WithEntityDetection()); err != nil {
		t.Fatalf("SendRichMarkdown with detection: %v", err)
	}
	rich2, _ := (*body2)["rich_message"].(map[string]any)
	if _, present := rich2["skip_entity_detection"]; present {
		t.Fatalf("rich_message = %v, want the field absent so Telegram detects entities", rich2)
	}
	if rich2["markdown"] != "see https://example.test" {
		t.Fatalf("markdown = %v", rich2["markdown"])
	}
}
