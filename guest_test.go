// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// A recorded guest_message decodes with its query id, the chat the bot was
// summoned into, and the photo the person replied to.
func TestAGuestMessageDecodes(t *testing.T) {
	raw := `{"update_id":11,"guest_message":{"message_id":0,"guest_query_id":"gq-1",
		"from":{"id":7,"is_bot":false,"first_name":"A","language_code":"uk"},
		"chat":{"id":-1001,"type":"supergroup","title":"Friends"},"date":1793000000,
		"text":"@framyin_bot make it snow","entities":[{"type":"mention","offset":0,"length":12}],
		"reply_to_message":{"message_id":40,"from":{"id":8,"is_bot":false,"first_name":"B"},
			"chat":{"id":-1001,"type":"supergroup"},"date":1792999000,
			"photo":[{"file_id":"small","file_unique_id":"s","width":90,"height":90},{"file_id":"big","file_unique_id":"b","width":1280,"height":1280}]}}}`
	var u Update
	if err := json.Unmarshal([]byte(raw), &u); err != nil {
		t.Fatal(err)
	}
	g := u.GuestMessage
	if g == nil || u.Message != nil || g.GuestQueryID != "gq-1" || g.From.ID != 7 || g.Chat.ID != -1001 || g.Text != "@framyin_bot make it snow" {
		t.Fatalf("guest message = %+v", g)
	}
	if r := g.ReplyToMessage; r == nil || len(r.Photo) != 2 || r.Photo[1].FileID != "big" {
		t.Fatalf("reply = %+v", r)
	}
}

// A message a guest bot sent says who summoned it, and getMe says whether
// Guest Mode is on.
func TestWhoSummonedAGuestBotDecodes(t *testing.T) {
	var m Message
	raw := `{"message_id":41,"from":{"id":99,"is_bot":true,"first_name":"framy"},"chat":{"id":-1001,"type":"supergroup"},"date":1,
		"guest_bot_caller_user":{"id":7,"is_bot":false,"first_name":"A"},"guest_bot_caller_chat":{"id":-1002,"type":"channel"}}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	if m.GuestBotCallerUser == nil || m.GuestBotCallerUser.ID != 7 || m.GuestBotCallerChat == nil || m.GuestBotCallerChat.ID != -1002 {
		t.Fatalf("message = %+v", m)
	}
	var me Me
	if err := json.Unmarshal([]byte(`{"id":99,"is_bot":true,"first_name":"framy","username":"framyin_bot","supports_guest_queries":true}`), &me); err != nil {
		t.Fatal(err)
	}
	if !me.SupportsGuestQueries {
		t.Fatalf("me = %+v", me)
	}
}

// The answer goes with the query's id and the result, and comes back as the
// inline message later edits use.
func TestAGuestQueryIsAnsweredWithAnInlineMessage(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"inline_message_id":"AAAB-inline"}}`)
	c, _ := newTestClient(t, handler)
	got, err := c.AnswerGuestQuery(context.Background(), "gq-1",
		InlineQueryResultPhoto{ID: "f1", PhotoURL: "https://files.test/a.jpg", ThumbnailURL: "https://files.test/a.t.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	if got != (SentGuestMessage{InlineMessageID: "AAAB-inline"}) {
		t.Fatalf("answer = %+v", got)
	}
	want := map[string]any{"guest_query_id": "gq-1", "result": map[string]any{
		"type": "photo", "id": "f1", "photo_url": "https://files.test/a.jpg", "thumbnail_url": "https://files.test/a.t.jpg"}}
	if *method != "answerGuestQuery" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

// What cannot be answered is refused before anything is sent, and an answer
// without the inline message is an error, since nothing could edit it.
func TestAGuestAnswerTelegramWouldRefuseIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	})
	photo := InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "x"}
	ctx := context.Background()
	if _, err := c.AnswerGuestQuery(ctx, "", photo); err == nil {
		t.Error("no query id was sent")
	}
	if _, err := c.AnswerGuestQuery(ctx, "gq-1", nil); err == nil {
		t.Error("no result was sent")
	}
	if sent {
		t.Fatal("a refused answer reached Telegram")
	}
	if _, err := c.AnswerGuestQuery(ctx, "gq-1", photo); err == nil || !sent {
		t.Errorf("an answer without inline_message_id: %v", err)
	}
}
