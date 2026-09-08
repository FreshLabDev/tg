// SPDX-License-Identifier: Apache-2.0
package tg

import "context"

// Ephemeral messages (Bot API 10.3) are visible to one person in a group. Two
// details decide whether a call succeeds:
//
//   - The receiver is named in ephemeral_message_parameters, not in a flat
//     receiver_user_id parameter, which is what 10.2 used.
//   - Telegram authorizes the send through what it replies to — either the
//     message being answered, within a 15-second window, or the callback query
//     of a tapped button. After that window a fresh ephemeral message is
//     refused, so long work must edit a placeholder posted immediately.

// SendEphemeralMessage answers a message so that only receiverUserID sees it.
func (c *Client) SendEphemeralMessage(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, text string, markup *InlineKeyboardMarkup) (Message, error) {
	req := map[string]any{
		"chat_id":                      chatID,
		"ephemeral_message_parameters": map[string]any{"receiver_user_id": receiverUserID},
		"text":                         text,
		"parse_mode":                   "HTML",
		"link_preview_options":         noLinkPreview,
		"reply_parameters": map[string]any{
			"ephemeral_message_id": ephemeralMessageID,
		},
	}
	return c.sendMessage(ctx, req, markup)
}

// SendEphemeralRichHTML delivers rich content visible only to receiverUserID.
// callbackQueryID ties the overlay to the button tap that asked for it; the
// public message stays in place because replace_callback_query_message is
// omitted.
func (c *Client) SendEphemeralRichHTML(ctx context.Context, chatID, receiverUserID int64, callbackQueryID, body string, markup *InlineKeyboardMarkup, opts ...RichOption) (Message, error) {
	ephemeral := map[string]any{"receiver_user_id": receiverUserID}
	if callbackQueryID != "" {
		ephemeral["callback_query_id"] = callbackQueryID
	}
	req := map[string]any{
		"chat_id":                      chatID,
		"rich_message":                 richHTML(body, opts...),
		"ephemeral_message_parameters": ephemeral,
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	return c.sendRichMessage(ctx, req)
}

// EditEphemeralMessageText replaces the text of an ephemeral message.
func (c *Client) EditEphemeralMessageText(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, text string, markup *InlineKeyboardMarkup) error {
	return c.editEphemeral(ctx, chatID, receiverUserID, ephemeralMessageID, map[string]any{
		"text":       text,
		"parse_mode": "HTML",
	}, markup)
}

// EditEphemeralRichHTML replaces an ephemeral placeholder with rich content.
// This is the only way to deliver more than 4096 characters privately: once
// the work has taken longer than Telegram's reply window, a fresh ephemeral
// message can no longer be sent.
func (c *Client) EditEphemeralRichHTML(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, body string, markup *InlineKeyboardMarkup, opts ...RichOption) error {
	return c.editEphemeral(ctx, chatID, receiverUserID, ephemeralMessageID, map[string]any{
		"rich_message": richHTML(body, opts...),
	}, markup)
}

func (c *Client) editEphemeral(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, content map[string]any, markup *InlineKeyboardMarkup) error {
	req := map[string]any{
		"chat_id":              chatID,
		"receiver_user_id":     receiverUserID,
		"ephemeral_message_id": ephemeralMessageID,
	}
	for k, v := range content {
		req[k] = v
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "editEphemeralMessageText", req, &resp); err != nil {
		if IsMessageNotModified(err) {
			return nil
		}
		return err
	}
	if !resp.OK {
		return okFalse("editEphemeralMessageText")
	}
	return nil
}

// DeleteEphemeralMessage removes an ephemeral message the bot posted.
func (c *Client) DeleteEphemeralMessage(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "deleteEphemeralMessage", map[string]any{
		"chat_id":              chatID,
		"receiver_user_id":     receiverUserID,
		"ephemeral_message_id": ephemeralMessageID,
	}, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("deleteEphemeralMessage")
	}
	return nil
}
