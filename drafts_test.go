// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

func TestADraftGoesOutWithStop(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)
	if err := c.SendMessageDraft(context.Background(), 7, 41, "<b>Generating</b> · 0:12", DraftOptions{ThreadID: 3, CanStop: true}); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"chat_id": float64(7), "draft_id": float64(41), "message_thread_id": float64(3),
		"text": "<b>Generating</b> · 0:12", "parse_mode": "HTML", "can_stop": true}
	if *method != "sendMessageDraft" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

// An empty draft is Telegram's "Thinking…" placeholder: no parse mode, and
// nothing unasked.
func TestAnEmptyDraftIsThePlaceholder(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)
	if err := c.SendMessageDraft(context.Background(), 7, 1, "", DraftOptions{}); err != nil {
		t.Fatal(err)
	}
	if want := (map[string]any{"chat_id": float64(7), "draft_id": float64(1), "text": ""}); !reflect.DeepEqual(*body, want) {
		t.Fatalf("body = %v", *body)
	}
}

func TestARichDraftKeepsOnStop(t *testing.T) {
	for name, send := range map[string]func(*Client) error{
		"html": func(c *Client) error {
			return c.SendRichDraftHTML(context.Background(), 7, 2, "<b>step 2</b>", DraftOptions{CanStop: true, KeepOnStop: true})
		},
		"markdown": func(c *Client) error {
			return c.SendRichDraftMarkdown(context.Background(), 7, 2, "**step 2**", DraftOptions{CanStop: true, KeepOnStop: true}, WithEntityDetection())
		},
	} {
		handler, method, body := capture(t, `{"ok":true,"result":true}`)
		c, _ := newTestClient(t, handler)
		if err := send(c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		rich := map[string]any{"html": "<b>step 2</b>", "skip_entity_detection": true}
		if name == "markdown" {
			rich = map[string]any{"markdown": "**step 2**"}
		}
		want := map[string]any{"chat_id": float64(7), "draft_id": float64(2), "rich_message": rich, "can_stop": true, "keep_on_stop": true}
		if *method != "sendRichMessageDraft" || !reflect.DeepEqual(*body, want) {
			t.Fatalf("%s: %s %v", name, *method, *body)
		}
	}
}

func TestADraftWithoutAnIDIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})
	if err := c.SendMessageDraft(context.Background(), 7, 0, "text", DraftOptions{}); err == nil {
		t.Error("a text draft without an id accepted")
	}
	if err := c.SendRichDraftHTML(context.Background(), 7, 0, "text", DraftOptions{}); err == nil {
		t.Error("a rich draft without an id accepted")
	}
	if sent {
		t.Fatal("a refused draft reached Telegram")
	}
}

// A recorded stopped_message_generation update decodes with its chat, topic
// and draft.
func TestAStopOnADraftDecodes(t *testing.T) {
	var u Update
	raw := `{"update_id":9,"stopped_message_generation":{"chat":{"id":7,"type":"private","first_name":"A"},"message_thread_id":3,"draft_id":41}}`
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	s := u.StoppedMessageGeneration
	if s == nil || s.Chat.ID != 7 || s.Chat.Type != ChatPrivate || s.ThreadID != 3 || s.DraftID != 41 || u.Message != nil {
		t.Fatalf("update = %+v", s)
	}
}
