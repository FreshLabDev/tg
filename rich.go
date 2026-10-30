// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
	"strconv"
)

// Rich messages (Bot API 10.1+) carry markup Telegram's classic parse_mode
// cannot express and are not capped at 4096 characters. Exactly one of html,
// markdown or blocks may be set.
//
// Entity detection is off in both builders: a transcript or a commit message
// full of phone numbers, hashtags and card-like digits should not turn into a
// field of links.

// RichOption adjusts how Telegram reads a rich payload.
type RichOption func(map[string]any)

// WithEntityDetection lets Telegram find links, hashtags, phone numbers and
// mentions in the payload itself. It is off by default because generated text
// -- a transcript, a diff, a commit message -- is full of digits and words
// that are not any of those. Turn it on for text a person wrote, or for
// markdown whose author expects a bare URL to become a link.
func WithEntityDetection() RichOption {
	return func(rich map[string]any) { delete(rich, "skip_entity_detection") }
}

func richHTML(body string, opts ...RichOption) map[string]any {
	return applyRichOptions(map[string]any{
		"html":                  body,
		"skip_entity_detection": true,
	}, opts)
}

// richMarkdown parses GFM. Prefer [richHTML] for anything generated: markdown
// would additionally interpret *, _, #, |, backticks and list-like lines that
// happen to appear in the payload.
func richMarkdown(body string, opts ...RichOption) map[string]any {
	return applyRichOptions(map[string]any{
		"markdown":              body,
		"skip_entity_detection": true,
	}, opts)
}

func applyRichOptions(rich map[string]any, opts []RichOption) map[string]any {
	for _, opt := range opts {
		if opt != nil {
			opt(rich)
		}
	}
	return rich
}

// SendRichHTML sends one rich message, optionally as a reply and inside a
// forum topic.
func (c *Client) SendRichHTML(ctx context.Context, chatID, replyTo int64, threadID int, body string, markup *InlineKeyboardMarkup, opts ...RichOption) (Message, error) {
	req := map[string]any{
		"chat_id":      chatID,
		"rich_message": richHTML(body, opts...),
	}
	if replyTo > 0 {
		req["reply_parameters"] = map[string]any{
			"message_id":                  replyTo,
			"allow_sending_without_reply": true,
		}
	}
	if threadID > 0 {
		req["message_thread_id"] = threadID
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	return c.sendRichMessage(ctx, req)
}

// SendRichMarkdown sends GFM. Only for content authored as markdown by a
// person or a source that owns its own escaping.
func (c *Client) SendRichMarkdown(ctx context.Context, chatID int64, markdown string, markup *InlineKeyboardMarkup, opts ...RichOption) (Message, error) {
	req := map[string]any{
		"chat_id":      chatID,
		"rich_message": richMarkdown(markdown, opts...),
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	return c.sendRichMessage(ctx, req)
}

// EditMessageRichHTML replaces an ordinary message with rich content. It is
// what turns a "working…" placeholder into the finished answer, so the user
// watches one message instead of waiting on an empty screen.
func (c *Client) EditMessageRichHTML(ctx context.Context, chatID, messageID int64, body string, markup *InlineKeyboardMarkup, opts ...RichOption) error {
	return c.editMessageRich(ctx, chatID, messageID, richHTML(body, opts...), markup)
}

// EditMessageRichMarkdown is [Client.EditMessageRichHTML] for GFM, with the same
// caveat as [Client.SendRichMarkdown]: only for content authored as markdown. It
// takes the same options, so [WithEntityDetection] lets Telegram find the links
// and mentions in a document a person wrote.
func (c *Client) EditMessageRichMarkdown(ctx context.Context, chatID, messageID int64, markdown string, markup *InlineKeyboardMarkup, opts ...RichOption) error {
	return c.editMessageRich(ctx, chatID, messageID, richMarkdown(markdown, opts...), markup)
}

// editMessageRich sends editMessageText with a rich_message. Like
// [Client.EditMessageText], it treats "message is not modified" as success.
func (c *Client) editMessageRich(ctx context.Context, chatID, messageID int64, rich map[string]any, markup *InlineKeyboardMarkup) error {
	req := map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"rich_message": rich,
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	err := c.post(ctx, "editMessageText", req, &resp)
	if IsMessageNotModified(err) {
		return nil
	}
	return err
}

func (c *Client) sendRichMessage(ctx context.Context, req map[string]any) (Message, error) {
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.post(ctx, "sendRichMessage", req, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse("sendRichMessage")
	}
	return resp.Result, nil
}

// RichMedia is a file a rich message's HTML shows by its ID:
// <img src="tg://photo?id=card"/> shows the media whose ID is "card". The
// file is a file_id, a URL or an upload, as in any InputMedia; Telegram reads
// the file and its properties and ignores a caption.
type RichMedia struct {
	// ID is 1 to 64 characters of A-Z, a-z, 0-9, _ and -.
	ID    string
	Media InputMedia
}

// RichOptions are the parameters of [Client.SendRich] besides the HTML.
type RichOptions struct {
	// ThreadID posts into a forum topic; zero stays in the main thread.
	ThreadID int
	// ReplyTo answers a message; zero sends a standalone one.
	ReplyTo int64
	// ReplyToEphemeral answers an ephemeral message, which is what lets a bot
	// that is not an administrator answer an ephemeral command privately.
	ReplyToEphemeral int64
	// Ephemeral shows the message to one member of a group.
	Ephemeral *Ephemeral
	Markup    *InlineKeyboardMarkup
	// Media are the files the HTML names. An upload makes the request
	// multipart; file_ids and URLs keep it JSON.
	Media []RichMedia
	Rich  []RichOption
}

// SendRich sends rich HTML with the media it shows, as a reply, into a topic
// or to one member of a group, as opts say.
func (c *Client) SendRich(ctx context.Context, chatID int64, html string, opts RichOptions) (Message, error) {
	rich := richHTML(html, opts.Rich...)
	var files []attachment
	if err := c.attachRichMedia(&files, rich, opts.Media); err != nil {
		return Message{}, err
	}
	req := mediaFields(chatID, CaptionOptions{
		ThreadID: opts.ThreadID, ReplyTo: opts.ReplyTo, ReplyToEphemeral: opts.ReplyToEphemeral,
		Ephemeral: opts.Ephemeral, Markup: opts.Markup,
	})
	req["rich_message"] = rich
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.submit(ctx, "sendRichMessage", req, files, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse("sendRichMessage")
	}
	return resp.Result, nil
}

// EditMessageRich replaces a message's content with rich HTML and the media
// it shows, and its keyboard with markup; nil leaves none. A text or a rich
// message takes it; a media message does not. "Message is not modified" is
// success.
func (c *Client) EditMessageRich(ctx context.Context, chatID, messageID int64, html string, media []RichMedia, markup *InlineKeyboardMarkup, opts ...RichOption) error {
	rich := richHTML(html, opts...)
	var files []attachment
	if err := c.attachRichMedia(&files, rich, media); err != nil {
		return err
	}
	req := map[string]any{"chat_id": chatID, "message_id": messageID, "rich_message": rich}
	if markup != nil {
		req["reply_markup"] = markup
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	err := c.submit(ctx, "editMessageText", req, files, &resp)
	if IsMessageNotModified(err) {
		return nil
	}
	if err == nil && !resp.OK {
		return okFalse("editMessageText")
	}
	return err
}

// EditEphemeralMessageRich is [Client.EditMessageRich] for an ephemeral
// message.
func (c *Client) EditEphemeralMessageRich(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, html string, media []RichMedia, markup *InlineKeyboardMarkup, opts ...RichOption) error {
	rich := richHTML(html, opts...)
	var files []attachment
	if err := c.attachRichMedia(&files, rich, media); err != nil {
		return err
	}
	return c.editEphemeralWith(ctx, "editEphemeralMessageText", chatID, receiverUserID, ephemeralMessageID, map[string]any{"rich_message": rich}, files, markup)
}

// attachRichMedia puts media into a rich payload: an upload becomes a part
// of its own, named rich0, rich1 and so on, and the item says attach:// to it.
func (c *Client) attachRichMedia(files *[]attachment, rich map[string]any, media []RichMedia) error {
	if len(media) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(media))
	for i, m := range media {
		if m.ID == "" || m.Media == nil {
			return fmt.Errorf("rich media %d needs an id and a file", i)
		}
		object, file := m.Media.item()
		value, err := c.attachInner(files, "rich"+strconv.Itoa(i), file)
		if err != nil {
			return fmt.Errorf("rich media %q: %w", m.ID, err)
		}
		object["media"] = value
		items = append(items, map[string]any{"id": m.ID, "media": object})
	}
	rich["media"] = items
	return nil
}
