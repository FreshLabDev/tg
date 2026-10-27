// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// recorded is one JSON request, decoded fresh. capture reuses one map for
// every request, which is right for a test that makes one call and wrong for a
// test that compares two.
type recorded struct {
	method string
	body   map[string]any
}

func record(result string) (http.HandlerFunc, *[]recorded) {
	var got []recorded
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		entry := recorded{method: r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], body: map[string]any{}}
		_ = json.Unmarshal(raw, &entry.body)
		got = append(got, entry)
		_, _ = w.Write([]byte(result))
	}, &got
}

// framy answers /f with an eyes reaction and swaps it for another when the
// result is posted. A reaction is set by the same call each time: it replaces
// whatever the bot put there before.
func TestSetMessageReactionReplacesTheBotsReaction(t *testing.T) {
	handler, got := record(`{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	for _, emoji := range []string{"👀", "🔥"} {
		if err := c.SetMessageReaction(context.Background(), -1001234567890, 412, []ReactionType{{Type: ReactionEmoji, Emoji: emoji}}); err != nil {
			t.Fatalf("SetMessageReaction(%s): %v", emoji, err)
		}
	}
	if len(*got) != 2 {
		t.Fatalf("requests = %d, want 2", len(*got))
	}
	for i, emoji := range []string{"👀", "🔥"} {
		r := (*got)[i]
		if r.method != "setMessageReaction" || r.body["chat_id"] != float64(-1001234567890) || r.body["message_id"] != float64(412) {
			t.Fatalf("request %d = %+v", i, r)
		}
		list, _ := r.body["reaction"].([]any)
		if len(list) != 1 {
			t.Fatalf("reaction = %v, want one", r.body["reaction"])
		}
		first, _ := list[0].(map[string]any)
		if first["type"] != "emoji" || first["emoji"] != emoji {
			t.Fatalf("reaction = %v, want the emoji %s", first, emoji)
		}
		if _, has := r.body["is_big"]; has {
			t.Fatal("is_big must be absent unless asked for")
		}
	}
}

func TestSetMessageReactionWithACustomEmoji(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.SetMessageReaction(context.Background(), 1, 2, []ReactionType{{Type: ReactionCustomEmoji, CustomEmojiID: "5368324170671202286"}})
	if err != nil {
		t.Fatalf("SetMessageReaction: %v", err)
	}
	list, _ := (*body)["reaction"].([]any)
	if len(list) != 1 {
		t.Fatalf("reaction = %v", (*body)["reaction"])
	}
	first, _ := list[0].(map[string]any)
	if first["type"] != "custom_emoji" || first["custom_emoji_id"] != "5368324170671202286" {
		t.Fatalf("reaction = %v", first)
	}
	if _, has := first["emoji"]; has {
		t.Fatalf("a custom emoji carries no emoji: %v", first)
	}
}

// Removing the bot's own reaction is the same method with nothing to set. The
// parameter is optional, and leaving it out is what the server reads as an
// empty list.
func TestSetMessageReactionWithNothingRemovesIt(t *testing.T) {
	for name, reactions := range map[string][]ReactionType{"nil": nil, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			handler, method, body := capture(t, `{"ok":true,"result":true}`)
			c, _ := newTestClient(t, handler)

			if err := c.SetMessageReaction(context.Background(), 1, 2, reactions); err != nil {
				t.Fatalf("SetMessageReaction: %v", err)
			}
			if *method != "setMessageReaction" {
				t.Fatalf("method = %q", *method)
			}
			if reaction, has := (*body)["reaction"]; has {
				t.Fatalf("reaction = %v, want none on the wire (and never null)", reaction)
			}
			if (*body)["chat_id"] != float64(1) || (*body)["message_id"] != float64(2) {
				t.Fatalf("body = %v", *body)
			}
		})
	}
}

// ✅ is not a reaction Telegram offers, and the answer is a plain 400 the
// caller has to be able to read.
func TestSetMessageReactionSurfacesARefusedEmoji(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: REACTION_INVALID"}`))
	})
	err := c.SetMessageReaction(context.Background(), 1, 2, []ReactionType{{Type: ReactionEmoji, Emoji: "✅"}})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v, want an *APIError with status 400", err)
	}
}

func TestDeleteMessageReactionNamesAUserOrAChat(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		handler, method, body := capture(t, `{"ok":true,"result":true}`)
		c, _ := newTestClient(t, handler)

		if err := c.DeleteMessageReaction(context.Background(), -1001234567890, 412, ReactionActor{UserID: 5550001}); err != nil {
			t.Fatalf("DeleteMessageReaction: %v", err)
		}
		if *method != "deleteMessageReaction" || (*body)["user_id"] != float64(5550001) ||
			(*body)["chat_id"] != float64(-1001234567890) || (*body)["message_id"] != float64(412) {
			t.Fatalf("method = %q, body = %v", *method, *body)
		}
		if _, has := (*body)["actor_chat_id"]; has {
			t.Fatal("actor_chat_id must be absent for a user")
		}
	})
	t.Run("chat", func(t *testing.T) {
		handler, _, body := capture(t, `{"ok":true,"result":true}`)
		c, _ := newTestClient(t, handler)

		if err := c.DeleteMessageReaction(context.Background(), -1001234567890, 412, ReactionActor{ActorChatID: -1009876543210}); err != nil {
			t.Fatalf("DeleteMessageReaction: %v", err)
		}
		if (*body)["actor_chat_id"] != float64(-1009876543210) {
			t.Fatalf("body = %v", *body)
		}
		if _, has := (*body)["user_id"]; has {
			t.Fatal("user_id must be absent for a chat")
		}
	})
}

// With neither id the server has nobody to remove, and with both it would have
// to guess. Neither is worth a request.
func TestDeleteMessageReactionRefusesAnUnclearActor(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an unclear actor must not reach the network")
	})
	for name, actor := range map[string]ReactionActor{
		"neither": {},
		"both":    {UserID: 1, ActorChatID: -2},
	} {
		if err := c.DeleteMessageReaction(context.Background(), 1, 2, actor); err == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
}

func TestDeleteMessagesDeletesABatch(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.DeleteMessages(context.Background(), -1001234567890, []int64{101, 102, 105}); err != nil {
		t.Fatalf("DeleteMessages: %v", err)
	}
	if *method != "deleteMessages" || (*body)["chat_id"] != float64(-1001234567890) {
		t.Fatalf("method = %q, body = %v", *method, *body)
	}
	ids, _ := (*body)["message_ids"].([]any)
	if len(ids) != 3 || ids[0] != float64(101) || ids[1] != float64(102) || ids[2] != float64(105) {
		t.Fatalf("message_ids = %v, want the three ids as numbers", (*body)["message_ids"])
	}
}

// Telegram takes 1 to 100 ids. A hundred is the largest batch that is sent.
func TestDeleteMessagesEnforcesTheBatchLimitsBeforeSending(t *testing.T) {
	var requests int
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})

	ids := make([]int64, 101)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	if err := c.DeleteMessages(context.Background(), 1, nil); err == nil {
		t.Fatal("want a refusal for no ids")
	}
	if err := c.DeleteMessages(context.Background(), 1, ids); err == nil {
		t.Fatal("want a refusal for 101 ids")
	}
	if requests != 0 {
		t.Fatalf("a refused batch still made %d requests", requests)
	}
	if err := c.DeleteMessages(context.Background(), 1, ids[:100]); err != nil {
		t.Fatalf("a batch of 100 must go through: %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}
