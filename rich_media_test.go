// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// An uploaded picture goes as a part of its own, and the rich payload names
// it by attach://, the way Telegram reads every other InputMedia.
func TestSendRichUploadsItsMediaAndNamesThem(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":7}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendRich(context.Background(), -100, `<img src="tg://photo?id=card"/><p>hi</p>`, RichOptions{
		ThreadID: 4, ReplyTo: 11,
		Ephemeral: &Ephemeral{ReceiverUserID: 5},
		Markup:    &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Go", CallbackData: "go"}}}},
		Media:     []RichMedia{{ID: "card", Media: InputMediaPhoto{Media: InputFileUpload{Filename: "card.png", Data: strings.NewReader("png")}}}},
	})
	if err != nil {
		t.Fatalf("SendRich: %v", err)
	}
	if msg.MessageID != 7 || got.method != "sendRichMessage" {
		t.Fatalf("message %d by %q", msg.MessageID, got.method)
	}
	if got.files["rich0"] != "png" || got.names["rich0"] != "card.png" {
		t.Fatalf("parts = %v %v, want the upload as rich0", got.files, got.names)
	}
	var rich struct {
		HTML  string `json:"html"`
		Skip  bool   `json:"skip_entity_detection"`
		Media []struct {
			ID    string         `json:"id"`
			Media map[string]any `json:"media"`
		} `json:"media"`
	}
	if err := json.Unmarshal([]byte(got.fields["rich_message"]), &rich); err != nil {
		t.Fatalf("rich_message = %q: %v", got.fields["rich_message"], err)
	}
	if !strings.Contains(rich.HTML, "tg://photo?id=card") || !rich.Skip {
		t.Fatalf("rich = %+v", rich)
	}
	if len(rich.Media) != 1 || rich.Media[0].ID != "card" || rich.Media[0].Media["type"] != "photo" || rich.Media[0].Media["media"] != "attach://rich0" {
		t.Fatalf("media = %+v", rich.Media)
	}
	if got.fields["message_thread_id"] != "4" || !strings.Contains(got.fields["ephemeral_message_parameters"], `"receiver_user_id":5`) {
		t.Fatalf("fields = %v", got.fields)
	}
	if !strings.Contains(got.fields["reply_parameters"], `"message_id":11`) || !strings.Contains(got.fields["reply_markup"], `"callback_data":"go"`) {
		t.Fatalf("reply %q, markup %q", got.fields["reply_parameters"], got.fields["reply_markup"])
	}
}

// A file Telegram already has costs no upload: the request stays JSON.
func TestSendRichByFileIDStaysJSON(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":8}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendRich(context.Background(), 42, `<img src="tg://photo?id=a"/>`, RichOptions{
		Media: []RichMedia{{ID: "a", Media: InputMediaPhoto{Media: InputFileString{Data: "AgACid"}}}},
	}); err != nil {
		t.Fatalf("SendRich: %v", err)
	}
	if *method != "sendRichMessage" {
		t.Fatalf("method = %q", *method)
	}
	rich, _ := (*body)["rich_message"].(map[string]any)
	media, _ := rich["media"].([]any)
	item, _ := media[0].(map[string]any)
	inner, _ := item["media"].(map[string]any)
	if item["id"] != "a" || inner["media"] != "AgACid" {
		t.Fatalf("media = %v", media)
	}
	if _, ok := (*body)["reply_parameters"]; ok {
		t.Fatal("no reply was asked for")
	}
}

func TestSendRichRefusesMediaWithoutAnID(t *testing.T) {
	called := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	_, err := c.SendRich(context.Background(), 42, "x", RichOptions{Media: []RichMedia{{Media: InputMediaPhoto{Media: InputFileString{Data: "id"}}}}})
	if err == nil || called {
		t.Fatalf("err = %v, called = %v", err, called)
	}
}

func TestEditMessageRichUploadsAndTreatsNotModifiedAsSuccess(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`)
	c, _ := newTestClient(t, handler)

	err := c.EditMessageRich(context.Background(), 42, 9, `<img src="tg://photo?id=card"/>`,
		[]RichMedia{{ID: "card", Media: InputMediaPhoto{Media: InputFileUpload{Filename: "c.png", Data: strings.NewReader("x")}}}}, nil)
	if err != nil {
		t.Fatalf("EditMessageRich: %v", err)
	}
	if got.method != "editMessageText" || got.fields["message_id"] != "9" || got.files["rich0"] != "x" {
		t.Fatalf("%q %v %v", got.method, got.fields, got.files)
	}
	if !strings.Contains(got.fields["rich_message"], `"attach://rich0"`) {
		t.Fatalf("rich_message = %q", got.fields["rich_message"])
	}
}

func TestEditEphemeralMessageRichNamesTheReceiver(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.EditEphemeralMessageRich(context.Background(), -100, 5, 77, "<p>x</p>", nil, nil); err != nil {
		t.Fatalf("EditEphemeralMessageRich: %v", err)
	}
	if *method != "editEphemeralMessageText" || (*body)["receiver_user_id"] != float64(5) || (*body)["ephemeral_message_id"] != float64(77) {
		t.Fatalf("%q %v", *method, *body)
	}
	rich, _ := (*body)["rich_message"].(map[string]any)
	if rich["html"] != "<p>x</p>" {
		t.Fatalf("rich_message = %v", rich)
	}
	if _, ok := rich["media"]; ok {
		t.Fatal("no media was given")
	}
}
