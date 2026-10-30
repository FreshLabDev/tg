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
	ID string `json:"id"`
	// PhotoURL is a JPEG of at most 5 MB, as Telegram documents it.
	PhotoURL     string `json:"photo_url"`
	ThumbnailURL string `json:"thumbnail_url"`
	PhotoWidth   int    `json:"photo_width,omitempty"`
	PhotoHeight  int    `json:"photo_height,omitempty"`
	// Title and Description appear in the picker only, never on the message
	// that gets sent.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Caption is HTML and does appear on the sent message.
	Caption               string                `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
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

// InlineQueryResultVideo sends a video Telegram fetches from VideoURL: an MP4
// file, or with MimeType "text/html" a page with a player, which goes as a
// link. Telegram requires the title and a JPEG thumbnail, so a card without
// either is refused before sending. An empty MimeType is "video/mp4".
type InlineQueryResultVideo struct {
	ID       string `json:"id"`
	VideoURL string `json:"video_url"`
	MimeType string `json:"mime_type"`
	// ThumbnailURL is a JPEG, shown in the picker and as the video's cover.
	ThumbnailURL          string                `json:"thumbnail_url"`
	Title                 string                `json:"title"`
	Description           string                `json:"description,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	VideoWidth            int                   `json:"video_width,omitempty"`
	VideoHeight           int                   `json:"video_height,omitempty"`
	VideoDuration         int                   `json:"video_duration,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultVideo) inlineQueryResult() {}

func (r InlineQueryResultVideo) MarshalJSON() ([]byte, error) {
	switch {
	case r.VideoURL == "":
		return nil, fmt.Errorf("a video result requires its URL")
	case r.Title == "":
		return nil, fmt.Errorf("a video result requires a title")
	case r.ThumbnailURL == "":
		return nil, fmt.Errorf("a video result requires a thumbnail")
	}
	if r.MimeType == "" {
		r.MimeType = "video/mp4"
	}
	type result InlineQueryResultVideo
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "video", ParseMode: captionParseMode(r.Caption), result: result(r)})
}

// InlineQueryResultMpeg4Gif sends a soundless MP4 animation Telegram fetches
// from Mpeg4URL. Telegram requires a thumbnail, so a card without one is
// refused before sending.
type InlineQueryResultMpeg4Gif struct {
	ID            string `json:"id"`
	Mpeg4URL      string `json:"mpeg4_url"`
	Mpeg4Width    int    `json:"mpeg4_width,omitempty"`
	Mpeg4Height   int    `json:"mpeg4_height,omitempty"`
	Mpeg4Duration int    `json:"mpeg4_duration,omitempty"`
	// ThumbnailURL is a JPEG, a GIF or an MP4; ThumbnailMimeType names which,
	// and empty is "image/jpeg".
	ThumbnailURL          string                `json:"thumbnail_url"`
	ThumbnailMimeType     string                `json:"thumbnail_mime_type,omitempty"`
	Title                 string                `json:"title,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultMpeg4Gif) inlineQueryResult() {}

func (r InlineQueryResultMpeg4Gif) MarshalJSON() ([]byte, error) {
	switch {
	case r.Mpeg4URL == "":
		return nil, fmt.Errorf("an MPEG-4 animation result requires its URL")
	case r.ThumbnailURL == "":
		return nil, fmt.Errorf("an MPEG-4 animation result requires a thumbnail")
	}
	type result InlineQueryResultMpeg4Gif
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "mpeg4_gif", ParseMode: captionParseMode(r.Caption), result: result(r)})
}

// The cached results send a file Telegram already has, by its file_id: a
// photo, a video or an MPEG-4 animation the bot sent or received before.
// Nothing is fetched, so they never fail on an unreachable URL, and the
// picker shows the file itself. A file_id is the bot's own: one another bot
// got for the same file does not work here.

// InlineQueryResultCachedPhoto sends a photo by its file_id.
type InlineQueryResultCachedPhoto struct {
	ID          string `json:"id"`
	PhotoFileID string `json:"photo_file_id"`
	// Title and Description appear in the picker only.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	// Caption is HTML and appears on the sent message.
	Caption               string `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool   `json:"show_caption_above_media,omitempty"`
	// ReplyMarkup is the keyboard of the sent message. One with a callback
	// button makes Telegram report the message's inline_message_id in
	// [ChosenInlineResult], and its presses come with it.
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	// InputMessageContent sends that instead of the photo.
	InputMessageContent InputMessageContent `json:"input_message_content,omitempty"`
}

func (InlineQueryResultCachedPhoto) inlineQueryResult() {}

func (r InlineQueryResultCachedPhoto) MarshalJSON() ([]byte, error) {
	type result InlineQueryResultCachedPhoto
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "photo", ParseMode: captionParseMode(r.Caption), result: result(r)})
}

// InlineQueryResultCachedVideo sends a video by its file_id. Telegram
// requires its Title, so a card without one is refused before sending.
type InlineQueryResultCachedVideo struct {
	ID                    string                `json:"id"`
	VideoFileID           string                `json:"video_file_id"`
	Title                 string                `json:"title"`
	Description           string                `json:"description,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultCachedVideo) inlineQueryResult() {}

func (r InlineQueryResultCachedVideo) MarshalJSON() ([]byte, error) {
	if r.Title == "" {
		return nil, fmt.Errorf("a cached video result requires a title")
	}
	type result InlineQueryResultCachedVideo
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "video", ParseMode: captionParseMode(r.Caption), result: result(r)})
}

// InlineQueryResultCachedMpeg4Gif sends a soundless MP4 animation by its
// file_id, which is what Telegram makes of a GIF. It has no description.
type InlineQueryResultCachedMpeg4Gif struct {
	ID                    string                `json:"id"`
	Mpeg4FileID           string                `json:"mpeg4_file_id"`
	Title                 string                `json:"title,omitempty"`
	Caption               string                `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool                  `json:"show_caption_above_media,omitempty"`
	ReplyMarkup           *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	InputMessageContent   InputMessageContent   `json:"input_message_content,omitempty"`
}

func (InlineQueryResultCachedMpeg4Gif) inlineQueryResult() {}

func (r InlineQueryResultCachedMpeg4Gif) MarshalJSON() ([]byte, error) {
	type result InlineQueryResultCachedMpeg4Gif
	return json.Marshal(struct {
		Type      string `json:"type"`
		ParseMode string `json:"parse_mode,omitempty"`
		result
	}{Type: "mpeg4_gif", ParseMode: captionParseMode(r.Caption), result: result(r)})
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
	// Button is shown above the results: it opens a Mini App or sends /start
	// with a parameter. Nil shows none.
	Button *InlineQueryResultsButton
}

// InlineQueryResultsButton is the button above an inline answer. It does
// exactly one thing: WebApp opens a Mini App, which can switch back to
// inline mode, or StartParameter opens the bot's chat with /start and that
// parameter (1 to 64 of A-Z, a-z, 0-9, _ and -), which is how a bot asks a
// person to sign in or connect something first.
type InlineQueryResultsButton struct {
	Text           string      `json:"text"`
	WebApp         *WebAppInfo `json:"web_app,omitempty"`
	StartParameter string      `json:"start_parameter,omitempty"`
}

func (b InlineQueryResultsButton) check() error {
	switch {
	case b.Text == "":
		return fmt.Errorf("an inline results button requires a text")
	case (b.WebApp == nil) == (b.StartParameter == ""):
		return fmt.Errorf("an inline results button takes exactly one of a web app and a start parameter")
	case b.WebApp != nil && b.WebApp.URL == "":
		return fmt.Errorf("an inline results button's web app requires a URL")
	case len(b.StartParameter) > 64 || !startParameter(b.StartParameter):
		return fmt.Errorf("an inline results button's start parameter is 1 to 64 of A-Z, a-z, 0-9, _ and -")
	}
	return nil
}

func startParameter(s string) bool {
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
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
	if answer.Button != nil {
		if err := answer.Button.check(); err != nil {
			return err
		}
		req["button"] = answer.Button
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

// A message sent through the bot in inline mode lives in somebody else's
// chat, which the bot may not be in. It is addressed by the
// inline_message_id that [ChosenInlineResult] and [CallbackQuery] carry, and
// an edit of it returns true rather than the message. A "message is not
// modified" answer is success for every one of them, as there is nothing to
// return either way.

// EditInlineMessageText replaces the text of an inline message, as HTML with
// link previews off, like [Client.EditMessageText].
func (c *Client) EditInlineMessageText(ctx context.Context, inlineMessageID, text string, markup *InlineKeyboardMarkup) error {
	req := map[string]any{"text": text, "parse_mode": "HTML", "link_preview_options": noLinkPreview}
	return c.editInline(ctx, "editMessageText", inlineMessageID, req, markup)
}

// EditInlineMessageTextWithPreview replaces the text of an inline message and
// lets Telegram show a preview of a link, which need not be in the text: the
// way an inline message carries a picture while it may still become text
// again, since a media message never can. A nil preview disables it, like
// [Client.SendTextWithPreview].
func (c *Client) EditInlineMessageTextWithPreview(ctx context.Context, inlineMessageID, text string, preview *LinkPreviewOptions, markup *InlineKeyboardMarkup) error {
	if preview == nil {
		preview = noLinkPreview
	}
	req := map[string]any{"text": text, "parse_mode": "HTML", "link_preview_options": preview}
	return c.editInline(ctx, "editMessageText", inlineMessageID, req, markup)
}

// EditInlineMessageMedia replaces the media of an inline message, which is
// how an inline card turns into a result once it is ready. Telegram takes no
// upload here, so the media is a file_id or a URL; an upload is refused
// before anything is sent.
func (c *Client) EditInlineMessageMedia(ctx context.Context, inlineMessageID string, media InputMedia, markup *InlineKeyboardMarkup) error {
	if media == nil {
		return fmt.Errorf("editMessageMedia requires media")
	}
	object, file := media.item()
	var files []attachment
	value, err := c.attachInner(&files, "media", file)
	if err != nil {
		return err
	}
	if len(files) > 0 {
		return fmt.Errorf("editMessageMedia of an inline message takes a file_id or a URL, not an upload")
	}
	object["media"] = value
	return c.editInline(ctx, "editMessageMedia", inlineMessageID, map[string]any{"media": object}, markup)
}

// EditInlineMessageCaption replaces the caption of an inline message, with
// the options of [Client.EditMessageCaption].
func (c *Client) EditInlineMessageCaption(ctx context.Context, inlineMessageID, caption string, opts EditCaptionOptions) error {
	req := map[string]any{"caption": caption}
	if caption != "" {
		req["parse_mode"] = "HTML"
	}
	if opts.ShowCaptionAboveMedia {
		req["show_caption_above_media"] = true
	}
	return c.editInline(ctx, "editMessageCaption", inlineMessageID, req, opts.Markup)
}

// EditInlineMessageReplyMarkup changes only the buttons of an inline message.
func (c *Client) EditInlineMessageReplyMarkup(ctx context.Context, inlineMessageID string, markup *InlineKeyboardMarkup) error {
	return c.editInline(ctx, "editMessageReplyMarkup", inlineMessageID, map[string]any{}, markup)
}

func (c *Client) editInline(ctx context.Context, method, inlineMessageID string, fields map[string]any, markup *InlineKeyboardMarkup) error {
	if inlineMessageID == "" {
		return fmt.Errorf("%s of an inline message requires its inline_message_id", method)
	}
	fields["inline_message_id"] = inlineMessageID
	if markup != nil {
		fields["reply_markup"] = markup
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	err := c.post(ctx, method, fields, &resp)
	if IsMessageNotModified(err) {
		return nil
	}
	if err == nil && !resp.OK {
		return okFalse(method)
	}
	return err
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
