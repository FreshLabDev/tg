// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestAnswerInlineQueryCarriesTheWholeAnswer(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.AnswerInlineQuery(context.Background(), "query-1", InlineAnswer{
		Results: []InlineQueryResult{
			InlineQueryResultPhoto{
				ID: "a", PhotoURL: "https://cdn.test/a.jpg", ThumbnailURL: "https://cdn.test/a-thumb.jpg",
				Title: "A picture",
			},
		},
		NextOffset: "1",
		CacheTime:  30,
		IsPersonal: true,
	})
	if err != nil {
		t.Fatalf("AnswerInlineQuery: %v", err)
	}
	if *method != "answerInlineQuery" {
		t.Fatalf("method = %q", *method)
	}
	if (*body)["inline_query_id"] != "query-1" {
		t.Fatalf("inline_query_id = %v", (*body)["inline_query_id"])
	}
	if (*body)["next_offset"] != "1" || (*body)["cache_time"] != float64(30) || (*body)["is_personal"] != true {
		t.Fatalf("body = %v", *body)
	}
	results, _ := (*body)["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %v", results)
	}
	card, _ := results[0].(map[string]any)
	if card["type"] != "photo" {
		t.Fatalf("a card without its discriminator is rejected whole: %v", card)
	}
	if _, has := card["parse_mode"]; has {
		t.Fatal("parse_mode with no caption to parse is noise in every capture")
	}
}

// An answer with no results at all shows the user nothing, so a bot says so
// with an article -- but the parameter itself is required either way, and an
// omitted one is a 400 rather than an empty list.
func TestAnswerInlineQueryAlwaysSendsResults(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.AnswerInlineQuery(context.Background(), "query-1", InlineAnswer{}); err != nil {
		t.Fatalf("AnswerInlineQuery: %v", err)
	}
	results, has := (*body)["results"].([]any)
	if !has || len(results) != 0 {
		t.Fatalf("results = %v, want an empty array", (*body)["results"])
	}
	for _, absent := range []string{"next_offset", "cache_time", "is_personal"} {
		if _, has := (*body)[absent]; has {
			t.Fatalf("%s must be absent when it was not asked for", absent)
		}
	}
}

func TestAnswerInlineQueryRefusesAnEmptyQueryID(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an answer addressed to nobody must not reach the network")
	})
	if err := c.AnswerInlineQuery(context.Background(), "", InlineAnswer{}); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestInlineQueryResultPhotoDeclaresHTMLOnlyWithACaption(t *testing.T) {
	raw, err := json.Marshal(InlineQueryResultPhoto{
		ID: "a", PhotoURL: "https://cdn.test/a.jpg", ThumbnailURL: "https://cdn.test/t.jpg",
		Caption: "<b>title</b>",
	})
	if err != nil {
		t.Fatal(err)
	}
	var card map[string]any
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatal(err)
	}
	if card["parse_mode"] != "HTML" {
		t.Fatalf("card = %s", raw)
	}
	if card["caption"] != "<b>title</b>" {
		t.Fatalf("card = %s", raw)
	}
	if _, has := card["description"]; has {
		t.Fatalf("an unset optional field must not become a null this package invented: %s", raw)
	}
}

func TestInlineQueryResultArticleCarriesItsContent(t *testing.T) {
	raw, err := json.Marshal(InlineQueryResultArticle{
		ID:                  "prompt",
		Title:               "Type to search",
		Description:         "images and videos",
		InputMessageContent: InputTextMessageContent{MessageText: "Type to search"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var card map[string]any
	if err := json.Unmarshal(raw, &card); err != nil {
		t.Fatal(err)
	}
	if card["type"] != "article" {
		t.Fatalf("card = %s", raw)
	}
	content, _ := card["input_message_content"].(map[string]any)
	if content["message_text"] != "Type to search" {
		t.Fatalf("content = %v", content)
	}
	if content["parse_mode"] != "HTML" {
		t.Fatal("text sent from a card is HTML like every other text this package sends")
	}
}

// The update kinds inline mode adds have to decode, or a bot polling for them
// silently handles nothing.
func TestUpdateDecodesInlineKinds(t *testing.T) {
	var update Update
	err := json.Unmarshal([]byte(`{"update_id":7,"inline_query":{"id":"q1","from":{"id":5,"is_bot":false,"first_name":"A","language_code":"uk"},"query":"cats","offset":"2","chat_type":"group"}}`), &update)
	if err != nil {
		t.Fatal(err)
	}
	if update.InlineQuery == nil || update.InlineQuery.Query != "cats" || update.InlineQuery.Offset != "2" {
		t.Fatalf("inline_query = %#v", update.InlineQuery)
	}
	if update.InlineQuery.From.LanguageCode != "uk" || update.InlineQuery.ChatType != "group" {
		t.Fatalf("inline_query = %#v", update.InlineQuery)
	}

	err = json.Unmarshal([]byte(`{"update_id":8,"chosen_inline_result":{"result_id":"a","from":{"id":5,"is_bot":false,"first_name":"A"},"query":"cats"}}`), &update)
	if err != nil {
		t.Fatal(err)
	}
	if update.ChosenInlineResult == nil || update.ChosenInlineResult.ResultID != "a" {
		t.Fatalf("chosen_inline_result = %#v", update.ChosenInlineResult)
	}
}

// The empty string is the useful value: it opens inline search with nothing
// typed. omitempty on a plain string would drop exactly that button.
func TestSwitchInlineQueryCurrentChatSurvivesItsEmptyValue(t *testing.T) {
	empty := ""
	raw, err := json.Marshal(InlineKeyboardButton{Text: "Search", SwitchInlineQueryCurrentChat: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"text":"Search","switch_inline_query_current_chat":""}` {
		t.Fatalf("button = %s", raw)
	}
	raw, _ = json.Marshal(InlineKeyboardButton{Text: "Plain", CallbackData: "x"})
	if strings.Contains(string(raw), "switch_inline_query") {
		t.Fatalf("an unset button must not carry the field: %s", raw)
	}
}

func TestAnswerCallbackQueryURLOpensALinkWithoutAToast(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.AnswerCallbackQueryURL(context.Background(), "cb-1", "https://t.me/vido?start=token"); err != nil {
		t.Fatalf("AnswerCallbackQueryURL: %v", err)
	}
	if *method != "answerCallbackQuery" {
		t.Fatalf("method = %q", *method)
	}
	if (*body)["url"] != "https://t.me/vido?start=token" {
		t.Fatalf("url = %v", (*body)["url"])
	}
	if _, has := (*body)["text"]; has {
		t.Fatal("a toast alongside the link is a second thing to dismiss")
	}
}

func TestSendTextWithPreviewKeepsThePreviewTheCallerAskedFor(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendTextWithPreview(context.Background(), 42, 3, "see https://example.test", &LinkPreviewOptions{}, nil); err != nil {
		t.Fatalf("SendTextWithPreview: %v", err)
	}
	preview, _ := (*body)["link_preview_options"].(map[string]any)
	if _, disabled := preview["is_disabled"]; disabled {
		t.Fatalf("link_preview_options = %v, want the preview left on", preview)
	}
	if (*body)["message_thread_id"] != float64(3) {
		t.Fatalf("message_thread_id = %v", (*body)["message_thread_id"])
	}

	handler, _, body = capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ = newTestClient(t, handler)
	if _, err := c.SendTextWithPreview(context.Background(), 42, 0, "text", nil, nil); err != nil {
		t.Fatalf("SendTextWithPreview: %v", err)
	}
	preview, _ = (*body)["link_preview_options"].(map[string]any)
	if preview["is_disabled"] != true {
		t.Fatal("nil must mean what the rest of the package does: no preview")
	}
}
