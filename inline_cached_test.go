// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// An answer of cached results and a button marshals as the Bot API documents
// answerInlineQuery, InlineQueryResultCached* and InlineQueryResultsButton.
func TestAnInlineAnswerOfCachedResultsAndAButton(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)
	press := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Again", CallbackData: "again"}}}}
	err := c.AnswerInlineQuery(context.Background(), "q", InlineAnswer{
		Results: []InlineQueryResult{
			InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "photo-id", Title: "A fox", Description: "Nano Banana", Caption: "<i>a fox</i>", ReplyMarkup: press},
			InlineQueryResultCachedVideo{ID: "v", VideoFileID: "video-id", Title: "Rain", ShowCaptionAboveMedia: true},
			InlineQueryResultCachedMpeg4Gif{ID: "g", Mpeg4FileID: "gif-id", Caption: "loop"},
			InlineQueryResultCachedPhoto{ID: "t", PhotoFileID: "photo-id", InputMessageContent: InputTextMessageContent{MessageText: "instead"}},
		},
		CacheTime:  5,
		IsPersonal: true,
		Button:     &InlineQueryResultsButton{Text: "Open framy", WebApp: &WebAppInfo{URL: "https://framy.test/app"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if *method != "answerInlineQuery" {
		t.Fatalf("method = %q", *method)
	}
	want := []any{
		map[string]any{"type": "photo", "id": "p", "photo_file_id": "photo-id", "title": "A fox", "description": "Nano Banana",
			"caption": "<i>a fox</i>", "parse_mode": "HTML",
			"reply_markup": map[string]any{"inline_keyboard": []any{[]any{map[string]any{"text": "Again", "callback_data": "again"}}}}},
		map[string]any{"type": "video", "id": "v", "video_file_id": "video-id", "title": "Rain", "show_caption_above_media": true},
		map[string]any{"type": "mpeg4_gif", "id": "g", "mpeg4_file_id": "gif-id", "caption": "loop", "parse_mode": "HTML"},
		map[string]any{"type": "photo", "id": "t", "photo_file_id": "photo-id",
			"input_message_content": map[string]any{"message_text": "instead", "parse_mode": "HTML"}},
	}
	if got := (*body)["results"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("results =\n%v\nwant\n%v", got, want)
	}
	if got, want := (*body)["button"], (map[string]any{"text": "Open framy", "web_app": map[string]any{"url": "https://framy.test/app"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("button = %v", got)
	}
}

func TestAStartParameterButtonGoesAsIs(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)
	if err := c.AnswerInlineQuery(context.Background(), "q", InlineAnswer{Button: &InlineQueryResultsButton{Text: "Sign in", StartParameter: "inline_sign-in"}}); err != nil {
		t.Fatal(err)
	}
	if got := (*body)["button"]; !reflect.DeepEqual(got, map[string]any{"text": "Sign in", "start_parameter": "inline_sign-in"}) {
		t.Fatalf("button = %v", got)
	}
}

// A button Telegram would refuse, and a cached video without its title, are
// refused before anything is sent.
func TestAnInlineAnswerTelegramWouldRefuseIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})
	for name, answer := range map[string]InlineAnswer{
		"no text":        {Button: &InlineQueryResultsButton{StartParameter: "x"}},
		"neither":        {Button: &InlineQueryResultsButton{Text: "x"}},
		"both":           {Button: &InlineQueryResultsButton{Text: "x", StartParameter: "x", WebApp: &WebAppInfo{URL: "https://framy.test"}}},
		"no url":         {Button: &InlineQueryResultsButton{Text: "x", WebApp: &WebAppInfo{}}},
		"bad parameter":  {Button: &InlineQueryResultsButton{Text: "x", StartParameter: "a b"}},
		"long parameter": {Button: &InlineQueryResultsButton{Text: "x", StartParameter: strings.Repeat("a", 65)}},
		"untitled video": {Results: []InlineQueryResult{InlineQueryResultCachedVideo{ID: "v", VideoFileID: "video-id"}}},
	} {
		if err := c.AnswerInlineQuery(context.Background(), "q", answer); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if sent {
		t.Fatal("a refused answer reached Telegram")
	}
}

// Each edit of an inline message names it by inline_message_id alone, takes
// true for an answer, and counts "not modified" as done.
func TestInlineMessagesAreEditedByTheirID(t *testing.T) {
	kb := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Open", URL: "https://framy.test"}}}}
	for name, c := range map[string]struct {
		edit   func(*Client) error
		method string
		want   map[string]any
	}{
		"text": {
			func(c *Client) error { return c.EditInlineMessageText(context.Background(), "im-1", "<b>done</b>", kb) },
			"editMessageText",
			map[string]any{"text": "<b>done</b>", "parse_mode": "HTML", "link_preview_options": map[string]any{"is_disabled": true}},
		},
		"text with a preview": {
			func(c *Client) error {
				preview := &LinkPreviewOptions{URL: "https://framy.test/card.jpg", PreferLargeMedia: true, ShowAboveText: true}
				return c.EditInlineMessageTextWithPreview(context.Background(), "im-1", "a fox", preview, kb)
			},
			"editMessageText",
			map[string]any{"text": "a fox", "parse_mode": "HTML", "link_preview_options": map[string]any{
				"url": "https://framy.test/card.jpg", "prefer_large_media": true, "show_above_text": true}},
		},
		"text with no preview": {
			func(c *Client) error {
				return c.EditInlineMessageTextWithPreview(context.Background(), "im-1", "a fox", nil, kb)
			},
			"editMessageText",
			map[string]any{"text": "a fox", "parse_mode": "HTML", "link_preview_options": map[string]any{"is_disabled": true}},
		},
		"media": {
			func(c *Client) error {
				return c.EditInlineMessageMedia(context.Background(), "im-1", InputMediaPhoto{Media: InputFileString{Data: "photo-id"}, Caption: "a fox"}, kb)
			},
			"editMessageMedia",
			map[string]any{"media": map[string]any{"type": "photo", "media": "photo-id", "caption": "a fox", "parse_mode": "HTML"}},
		},
		"caption": {
			func(c *Client) error {
				return c.EditInlineMessageCaption(context.Background(), "im-1", "a fox", EditCaptionOptions{Markup: kb, ShowCaptionAboveMedia: true})
			},
			"editMessageCaption",
			map[string]any{"caption": "a fox", "parse_mode": "HTML", "show_caption_above_media": true},
		},
		"buttons": {
			func(c *Client) error { return c.EditInlineMessageReplyMarkup(context.Background(), "im-1", kb) },
			"editMessageReplyMarkup",
			map[string]any{},
		},
	} {
		handler, method, body := capture(t, `{"ok":true,"result":true}`)
		client, _ := newTestClient(t, handler)
		if err := c.edit(client); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		c.want["inline_message_id"] = "im-1"
		c.want["reply_markup"] = map[string]any{"inline_keyboard": []any{[]any{map[string]any{"text": "Open", "url": "https://framy.test"}}}}
		if *method != c.method || !reflect.DeepEqual(*body, c.want) {
			t.Fatalf("%s: %s %v\nwant %s %v", name, *method, *body, c.method, c.want)
		}

		notModified := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`))
		}
		client, _ = newTestClient(t, notModified)
		if err := c.edit(client); err != nil {
			t.Fatalf("%s: not modified: %v", name, err)
		}
	}
}

// An inline message takes no upload, and an edit without an id goes nowhere.
func TestAnInlineEditTelegramWouldRefuseIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})
	ctx := context.Background()
	upload := InputMediaPhoto{Media: InputFileUpload{Filename: "a.png", Data: strings.NewReader("png")}}
	if err := c.EditInlineMessageMedia(ctx, "im-1", upload, nil); err == nil {
		t.Error("an upload accepted")
	}
	if err := c.EditInlineMessageText(ctx, "", "text", nil); err == nil {
		t.Error("an edit without an id accepted")
	}
	if err := c.EditInlineMessageTextWithPreview(ctx, "", "text", nil, nil); err == nil {
		t.Error("an edit with a preview and without an id accepted")
	}
	if err := c.EditInlineMessageMedia(ctx, "im-1", nil, nil); err == nil {
		t.Error("an edit without media accepted")
	}
	if sent {
		t.Fatal("a refused edit reached Telegram")
	}
}

// A press on a button of an inline message comes without a message, with
// its inline_message_id; a press on the bot's own message has the message.
func TestACallbackFromAnInlineMessageHasNoMessage(t *testing.T) {
	var inline, own Update
	if err := json.Unmarshal([]byte(`{"update_id":1,"callback_query":{"id":"cq","from":{"id":7,"is_bot":false,"first_name":"A"},
		"inline_message_id":"AgAAAAc","chat_instance":"-42","data":"again"}}`), &inline); err != nil {
		t.Fatal(err)
	}
	if cq := inline.Callback; cq == nil || cq.Message != nil || cq.InlineMessageID != "AgAAAAc" || cq.Data != "again" || cq.ChatInstance != "-42" {
		t.Fatalf("inline = %+v", inline.Callback)
	}
	if err := json.Unmarshal([]byte(`{"update_id":2,"callback_query":{"id":"cq","from":{"id":7,"is_bot":false,"first_name":"A"},
		"message":{"message_id":5,"date":1,"chat":{"id":-100,"type":"supergroup"}},"chat_instance":"-42","data":"again"}}`), &own); err != nil {
		t.Fatal(err)
	}
	if cq := own.Callback; cq == nil || cq.Message == nil || cq.Message.MessageID != 5 || cq.Message.Chat.ID != -100 || cq.InlineMessageID != "" {
		t.Fatalf("own = %+v", own.Callback)
	}
}
