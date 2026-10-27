// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Bot API 10.3 moved the receiver into ephemeral_message_parameters. Sending
// the flat 10.2 parameter instead is accepted by nothing and fails silently
// for a non-admin bot.
func TestSendEphemeralMessageUsesTheNestedParameters(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":9,"ephemeral_message_id":4}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendEphemeralMessage(context.Background(), 42, 77, 4, "only for you", nil)
	if err != nil {
		t.Fatalf("SendEphemeralMessage: %v", err)
	}
	if msg.EphemeralMessageID != 4 {
		t.Fatalf("ephemeral_message_id = %d", msg.EphemeralMessageID)
	}
	if *method != "sendMessage" {
		t.Fatalf("method = %q", *method)
	}
	if _, flat := (*body)["receiver_user_id"]; flat {
		t.Fatal("receiver_user_id at the top level is the removed 10.2 shape")
	}
	params, _ := (*body)["ephemeral_message_parameters"].(map[string]any)
	if params["receiver_user_id"] != float64(77) {
		t.Fatalf("ephemeral_message_parameters = %v", params)
	}
	reply, _ := (*body)["reply_parameters"].(map[string]any)
	if reply["ephemeral_message_id"] != float64(4) {
		t.Fatalf("reply_parameters = %v, want the reply that authorizes the send", reply)
	}
}

func TestSendEphemeralRichHTMLCarriesTheCallbackQuery(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":9}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendEphemeralRichHTML(context.Background(), 42, 77, "cb-1", "<i>hi</i>", nil); err != nil {
		t.Fatalf("SendEphemeralRichHTML: %v", err)
	}
	if *method != "sendRichMessage" {
		t.Fatalf("method = %q", *method)
	}
	params, _ := (*body)["ephemeral_message_parameters"].(map[string]any)
	if params["callback_query_id"] != "cb-1" {
		t.Fatalf("callback_query_id = %v", params["callback_query_id"])
	}
	if _, replaces := params["replace_callback_query_message"]; replaces {
		t.Fatal("the public message must stay in place")
	}
}

func TestEditEphemeralRichHTMLSendsTheFlatReceiver(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.EditEphemeralRichHTML(context.Background(), 42, 77, 4, "done", nil); err != nil {
		t.Fatalf("EditEphemeralRichHTML: %v", err)
	}
	if *method != "editEphemeralMessageText" {
		t.Fatalf("method = %q", *method)
	}
	// editEphemeralMessageText addresses an existing message, so here the
	// receiver really is a flat parameter — unlike the send above.
	if (*body)["receiver_user_id"] != float64(77) || (*body)["ephemeral_message_id"] != float64(4) {
		t.Fatalf("body = %v", *body)
	}
	rich, _ := (*body)["rich_message"].(map[string]any)
	if rich["html"] != "done" {
		t.Fatalf("rich_message = %v", rich)
	}
}

// Every text this package sends suppresses Telegram's own link preview, and an
// ephemeral edit is the one place that had been forgotten. An About panel
// carrying a repository link renders the preview card under it otherwise --
// in a group, where the panel is meant to be the quiet option.
func TestEphemeralTextSuppressesLinkPreviewsLikeEveryOtherSend(t *testing.T) {
	for _, test := range []struct {
		name string
		call func(*Client) error
	}{
		{"send", func(c *Client) error {
			_, err := c.SendEphemeralMessage(context.Background(), 1, 2, 3, `see <a href="https://github.com/FreshLabDev/tg">the source</a>`, nil)
			return err
		}},
		{"edit", func(c *Client) error {
			return c.EditEphemeralMessageText(context.Background(), 1, 2, 3, `see <a href="https://github.com/FreshLabDev/tg">the source</a>`, nil)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, _, body := capture(t, `{"ok":true,"result":{"message_id":5}}`)
			c, _ := newTestClient(t, handler)
			if err := test.call(c); err != nil {
				t.Fatalf("%s: %v", test.name, err)
			}
			preview, _ := (*body)["link_preview_options"].(map[string]any)
			if preview["is_disabled"] != true {
				t.Fatalf("link_preview_options = %v, want the preview off", (*body)["link_preview_options"])
			}
		})
	}
}

// A bot that is not an admin may answer a button press with an ephemeral
// photo, shown to the one who pressed in place of the message they pressed
// on.
func TestSendPhotoCanBeEphemeral(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":0,"ephemeral_message_id":5}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendPhoto(context.Background(), 42, InputFileString{Data: "file-1"}, CaptionOptions{
		ReplyTo:   10,
		Caption:   "only for you",
		Ephemeral: &Ephemeral{ReceiverUserID: 77, CallbackQueryID: "cb-1", ReplaceCallbackQueryMessage: true},
	})
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if *method != "sendPhoto" || msg.EphemeralMessageID != 5 {
		t.Fatalf("method %q, message %+v", *method, msg)
	}
	params, _ := (*body)["ephemeral_message_parameters"].(map[string]any)
	if params["receiver_user_id"] != float64(77) || params["callback_query_id"] != "cb-1" || params["replace_callback_query_message"] != true {
		t.Fatalf("ephemeral_message_parameters = %v", params)
	}
	if _, flat := (*body)["receiver_user_id"]; flat {
		t.Fatal("receiver_user_id at the top level is the removed 10.2 shape")
	}
	// An ephemeral reply needs the message it answers to be there, so the
	// flag the public send carries is left out.
	reply, _ := (*body)["reply_parameters"].(map[string]any)
	if reply["message_id"] != float64(10) || len(reply) != 1 {
		t.Fatalf("reply_parameters = %v", reply)
	}
}

// An ephemeral command authorizes the answer through the reply to it.
func TestAnEphemeralReplyNamesTheEphemeralMessage(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":0,"ephemeral_message_id":6}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendAnimation(context.Background(), 42, InputFileString{Data: "loop"}, AnimationOptions{CaptionOptions: CaptionOptions{
		ReplyToEphemeral: 3,
		Ephemeral:        &Ephemeral{ReceiverUserID: 77},
	}}); err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	reply, _ := (*body)["reply_parameters"].(map[string]any)
	if reply["ephemeral_message_id"] != float64(3) || len(reply) != 1 {
		t.Fatalf("reply_parameters = %v", reply)
	}
	params, _ := (*body)["ephemeral_message_parameters"].(map[string]any)
	if len(params) != 1 || params["receiver_user_id"] != float64(77) {
		t.Fatalf("ephemeral_message_parameters = %v, want the receiver alone", params)
	}
}

func TestAnUploadedEphemeralPhotoCarriesItsParametersAsJSON(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":0,"ephemeral_message_id":7}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendPhoto(context.Background(), 42, InputFileUpload{Filename: "card.png", Data: bytes.NewReader([]byte("png"))}, CaptionOptions{
		Ephemeral: &Ephemeral{ReceiverUserID: 77},
	}); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if got.files["photo"] != "png" || got.fields["ephemeral_message_parameters"] != `{"receiver_user_id":77}` {
		t.Fatalf("fields %v, files %v", got.fields, got.files)
	}
}

// The edit uploads a new picture the way editMessageMedia does, and addresses
// the message by the flat receiver.
func TestEditEphemeralMessageMediaUploadsTheNewPhoto(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	markup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Generate", CallbackData: "go"}}}}
	err := c.EditEphemeralMessageMedia(context.Background(), 42, 77, 5, InputMediaPhoto{
		Media:   InputFileUpload{Filename: "card2.png", Data: bytes.NewReader([]byte("png2"))},
		Caption: "<b>new</b>",
	}, markup)
	if err != nil {
		t.Fatalf("EditEphemeralMessageMedia: %v", err)
	}
	if got.method != "editEphemeralMessageMedia" {
		t.Fatalf("method = %q", got.method)
	}
	if got.fields["chat_id"] != "42" || got.fields["receiver_user_id"] != "77" || got.fields["ephemeral_message_id"] != "5" {
		t.Fatalf("fields = %v", got.fields)
	}
	var media map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
		t.Fatalf("media = %q: %v", got.fields["media"], err)
	}
	attach, _ := media["media"].(string)
	if media["type"] != "photo" || media["caption"] != "<b>new</b>" || !strings.HasPrefix(attach, "attach://") {
		t.Fatalf("media = %v", media)
	}
	if got.files[strings.TrimPrefix(attach, "attach://")] != "png2" {
		t.Fatalf("files = %v", got.files)
	}
	if !strings.Contains(got.fields["reply_markup"], `"callback_data":"go"`) {
		t.Fatalf("reply_markup = %q", got.fields["reply_markup"])
	}
}

func TestEditEphemeralMessageCaptionAndButtons(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.EditEphemeralMessageCaption(context.Background(), 42, 77, 5, "≈ $0.04", EditCaptionOptions{ShowCaptionAboveMedia: true}); err != nil {
		t.Fatalf("EditEphemeralMessageCaption: %v", err)
	}
	if *method != "editEphemeralMessageCaption" || (*body)["caption"] != "≈ $0.04" || (*body)["parse_mode"] != "HTML" ||
		(*body)["show_caption_above_media"] != true || (*body)["receiver_user_id"] != float64(77) || (*body)["ephemeral_message_id"] != float64(5) {
		t.Fatalf("%s %v", *method, *body)
	}
	if err := c.EditEphemeralMessageReplyMarkup(context.Background(), 42, 77, 5, nil); err != nil {
		t.Fatalf("EditEphemeralMessageReplyMarkup: %v", err)
	}
	if _, has := (*body)["reply_markup"]; *method != "editEphemeralMessageReplyMarkup" || has {
		t.Fatalf("%s %v", *method, *body)
	}
}

// A press that changes nothing is not a failure, as for every other edit.
func TestEphemeralEditsTreatNotModifiedAsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: message is not modified"}`))
	})
	ctx := context.Background()
	if err := c.EditEphemeralMessageMedia(ctx, 1, 2, 3, InputMediaPhoto{Media: InputFileString{Data: "id"}}, nil); err != nil {
		t.Fatalf("media: %v", err)
	}
	if err := c.EditEphemeralMessageCaption(ctx, 1, 2, 3, "x", EditCaptionOptions{}); err != nil {
		t.Fatalf("caption: %v", err)
	}
	if err := c.EditEphemeralMessageReplyMarkup(ctx, 1, 2, 3, nil); err != nil {
		t.Fatalf("markup: %v", err)
	}
	if err := c.EditEphemeralMessageMedia(ctx, 1, 2, 3, nil, nil); err == nil {
		t.Fatal("an edit without media was sent")
	}
}
