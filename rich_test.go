// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
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
