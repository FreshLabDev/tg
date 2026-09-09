// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"fmt"
)

// Inline mode is the surface where a bot answers inside somebody else's chat,
// without being a member of it. Two things about it decide whether an answer
// appears at all, and neither produces a useful error:
//
//   - Telegram fetches every media and thumbnail URL itself, over HTTPS. One
//     unreachable URL rejects the whole answer, not just its card.
//   - answerInlineQuery has a window of a few seconds. A late answer is
//     accepted and shown to nobody, so the work behind a query belongs in
//     front of it, not after it.

// InlineQuery is what a user typed after the bot's @username, anywhere.
type InlineQuery struct {
	ID   string `json:"id"`
	From User   `json:"from"`
	// Query is the raw text after the @username, which the bot must treat as
	// untrusted input like any other message.
	Query string `json:"query"`
	// Offset is what the previous answer returned as its next offset, empty on
	// the first page. Its meaning is the bot's own; Telegram only echoes it.
	Offset string `json:"offset"`
	// ChatType is where the query was typed: "sender", "private", "group",
	// "supergroup" or "channel". It is absent for a query from a chosen inline
	// result's own chat.
	ChatType string `json:"chat_type,omitempty"`
}

// ChosenInlineResult reports that a user actually sent one of the answers. It
// only arrives when inline feedback is enabled for the bot in BotFather, and
// it is the only way to learn which card was picked.
type ChosenInlineResult struct {
	ResultID string `json:"result_id"`
	From     User   `json:"from"`
	Query    string `json:"query"`
	// InlineMessageID identifies the sent message for later edits. It is
	// present only when the result carried a keyboard.
	InlineMessageID string `json:"inline_message_id,omitempty"`
}

// InlineQueryResult is one card in an answer. The concrete types below carry
// their own "type" discriminator, which is why they marshal themselves.
type InlineQueryResult interface {
	inlineQueryResult()
}

// InlineQueryResultPhoto shows an image. PhotoURL is what gets sent when the
// card is picked; ThumbnailURL is what the picker shows while scrolling, and
// should be the smaller of the two.
type InlineQueryResultPhoto struct {
	ID           string `json:"id"`
	PhotoURL     string `json:"photo_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	// Title and Description appear in the picker only, never on the message
	// that gets sent.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Caption is HTML and does appear on the sent message.
	Caption     string                `json:"caption,omitempty"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

func (InlineQueryResultPhoto) inlineQueryResult() {}

func (r InlineQueryResultPhoto) MarshalJSON() ([]byte, error) {
	type result InlineQueryResultPhoto
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "photo", ParseMode: captionParseMode(r.Caption), result: result(r)})
}

// InlineQueryResultArticle is a text card. It is what a bot answers with when
// it has nothing to show yet -- a prompt, or an empty-result notice -- because
// an inline answer with no results at all shows the user nothing whatsoever.
type InlineQueryResultArticle struct {
	ID                  string              `json:"id"`
	Title               string              `json:"title"`
	Description         string              `json:"description,omitempty"`
	InputMessageContent InputMessageContent `json:"input_message_content"`
	URL                 string              `json:"url,omitempty"`
	ThumbnailURL        string              `json:"thumbnail_url,omitempty"`

	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

func (InlineQueryResultArticle) inlineQueryResult() {}

func (r InlineQueryResultArticle) MarshalJSON() ([]byte, error) {
	type result InlineQueryResultArticle
	return json.Marshal(struct {
		Type string `json:"type"`
		result
	}{Type: "article", result: result(r)})
}

// InputMessageContent is what a card puts in the chat when it is picked, for
// the card types that do not send media.
type InputMessageContent interface {
	inputMessageContent()
}

// InputTextMessageContent sends HTML text, like every other text in this
// package. LinkPreview is nil-safe: leaving it unset keeps Telegram's default,
// which is to preview the first link it finds.
type InputTextMessageContent struct {
	MessageText string              `json:"message_text"`
	LinkPreview *LinkPreviewOptions `json:"link_preview_options,omitempty"`
}

func (InputTextMessageContent) inputMessageContent() {}

func (c InputTextMessageContent) MarshalJSON() ([]byte, error) {
	type content InputTextMessageContent
	return json.Marshal(struct {
		ParseMode string `json:"parse_mode"`
		content
	}{ParseMode: "HTML", content: content(c)})
}

// InlineAnswer is one reply to an inline query.
type InlineAnswer struct {
	Results []InlineQueryResult
	// NextOffset is echoed back in the next [InlineQuery] when the user
	// scrolls past the end. An empty one says this is the last page; anything
	// else must eventually become empty, or scrolling never stops.
	NextOffset string
	// CacheTime is how long Telegram may reuse this answer, in seconds. Zero
	// means Telegram's own default of 300, which is a long time for a personal
	// answer to sit in a cache.
	CacheTime int
	// IsPersonal keeps the cached answer to the user who asked. Anything that
	// depends on who is asking -- their language, their permissions, a token
	// bound to them -- must set it.
	IsPersonal bool
}

// AnswerInlineQuery replies to an inline query. Telegram accepts one answer
// per query and ignores the rest, so a bot that debounces keystrokes must drop
// the superseded work before it answers, not after.
func (c *Client) AnswerInlineQuery(ctx context.Context, inlineQueryID string, answer InlineAnswer) error {
	if inlineQueryID == "" {
		return fmt.Errorf("answerInlineQuery requires a query id")
	}
	req := map[string]any{
		"inline_query_id": inlineQueryID,
		// Telegram requires the parameter, and an empty array is a legitimate
		// answer meaning "nothing found".
		"results": answer.Results,
	}
	if answer.Results == nil {
		req["results"] = []InlineQueryResult{}
	}
	if answer.NextOffset != "" {
		req["next_offset"] = answer.NextOffset
	}
	if answer.CacheTime > 0 {
		req["cache_time"] = answer.CacheTime
	}
	if answer.IsPersonal {
		req["is_personal"] = true
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "answerInlineQuery", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("answerInlineQuery")
	}
	return nil
}

// captionParseMode declares HTML only when there is a caption to parse.
// Sending parse_mode with no caption is harmless but shows up in every capture
// of every card, which makes a real difference unreadable.
func captionParseMode(caption string) string {
	if caption == "" {
		return ""
	}
	return "HTML"
}
