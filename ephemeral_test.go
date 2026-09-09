// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
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
