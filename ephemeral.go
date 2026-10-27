// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
)

// Ephemeral messages (Bot API 10.3) are visible to one person in a group. Two
// details decide whether a call succeeds:
//
//   - The receiver is named in ephemeral_message_parameters, not in a flat
//     receiver_user_id parameter, which is what 10.2 used.
//   - Telegram authorizes the send through what it replies to — either the
//     message being answered, within a 15-second window, or the callback query
//     of a tapped button. After that window a fresh ephemeral message is
//     refused, so long work must edit a placeholder posted immediately.

// Ephemeral addresses a send to one member of a group: SendPhoto,
// SendAnimation, SendVideo, SendAudio and SendDocument take it in their
// options. A bot that is an administrator of the chat may send one at any
// time. Any other bot needs what authorizes it, within 15 seconds: the press
// in CallbackQueryID, or the incoming ephemeral message in
// CaptionOptions.ReplyToEphemeral.
type Ephemeral struct {
	ReceiverUserID int64
	// CallbackQueryID is the button press the message answers.
	CallbackQueryID string
	// ReplaceCallbackQueryMessage shows the message to the receiver in place
	// of the one whose button was pressed; the others keep seeing that one. It
	// must be false for a press on an ephemeral message, which is edited with
	// the EditEphemeral methods instead.
	ReplaceCallbackQueryMessage bool
}

func (e *Ephemeral) fields() map[string]any {
	params := map[string]any{"receiver_user_id": e.ReceiverUserID}
	if e.CallbackQueryID != "" {
		params["callback_query_id"] = e.CallbackQueryID
	}
	if e.ReplaceCallbackQueryMessage {
		params["replace_callback_query_message"] = true
	}
	return params
}

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

// EditEphemeralMessageText replaces the text of an ephemeral message. Like
// every other text this package sends, it suppresses Telegram's own preview of
// a link inside it: an About panel carrying a repository link would otherwise
// grow a preview card under it, and in a group that card is the loudest thing
// on screen.
func (c *Client) EditEphemeralMessageText(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, text string, markup *InlineKeyboardMarkup) error {
	return c.editEphemeral(ctx, chatID, receiverUserID, ephemeralMessageID, map[string]any{
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": noLinkPreview,
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
	return c.editEphemeralWith(ctx, "editEphemeralMessageText", chatID, receiverUserID, ephemeralMessageID, content, nil, markup)
}

// EditEphemeralMessageMedia replaces the file of an ephemeral message, as
// [Client.EditMessageMedia] does for a message in the chat: a file_id, a URL,
// a path on a --local server or an upload, with the caption and its options
// on the InputMedia. Telegram returns no message, so the file_id of an upload
// is not reported. A "message is not modified" answer is success.
func (c *Client) EditEphemeralMessageMedia(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, media InputMedia, markup *InlineKeyboardMarkup) error {
	if media == nil {
		return fmt.Errorf("editEphemeralMessageMedia requires media")
	}
	object, file := media.item()
	var files []attachment
	value, err := c.attachInner(&files, "media", file)
	if err != nil {
		return err
	}
	object["media"] = value
	return c.editEphemeralWith(ctx, "editEphemeralMessageMedia", chatID, receiverUserID, ephemeralMessageID, map[string]any{"media": object}, files, markup)
}

// EditEphemeralMessageCaption replaces the caption of an ephemeral message.
// The caption is HTML and an empty one removes it, as in
// [Client.EditMessageCaption]; opts.Markup is the keyboard after the edit.
// A "message is not modified" answer is success.
func (c *Client) EditEphemeralMessageCaption(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, caption string, opts EditCaptionOptions) error {
	content := map[string]any{"caption": caption}
	if caption != "" {
		content["parse_mode"] = "HTML"
	}
	if opts.ShowCaptionAboveMedia {
		content["show_caption_above_media"] = true
	}
	return c.editEphemeralWith(ctx, "editEphemeralMessageCaption", chatID, receiverUserID, ephemeralMessageID, content, nil, opts.Markup)
}

// EditEphemeralMessageReplyMarkup changes only the buttons of an ephemeral
// message; nil takes them away. A "message is not modified" answer is
// success.
func (c *Client) EditEphemeralMessageReplyMarkup(ctx context.Context, chatID, receiverUserID, ephemeralMessageID int64, markup *InlineKeyboardMarkup) error {
	return c.editEphemeralWith(ctx, "editEphemeralMessageReplyMarkup", chatID, receiverUserID, ephemeralMessageID, nil, nil, markup)
}

// editEphemeralWith runs one of the editEphemeralMessage methods. They address
// the message by a flat receiver_user_id, unlike a send, and answer true.
func (c *Client) editEphemeralWith(ctx context.Context, method string, chatID, receiverUserID, ephemeralMessageID int64, content map[string]any, files []attachment, markup *InlineKeyboardMarkup) error {
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
	if err := c.submit(ctx, method, req, files, &resp); err != nil {
		if IsMessageNotModified(err) {
			return nil
		}
		return err
	}
	if !resp.OK {
		return okFalse(method)
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
