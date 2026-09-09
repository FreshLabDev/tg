// SPDX-License-Identifier: Apache-2.0
package tg

import "context"

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
	req := map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"rich_message": richHTML(body, opts...),
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
