// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"time"
)

// noLinkPreview replaces the removed disable_web_page_preview parameter.
var noLinkPreview = map[string]any{"is_disabled": true}

func (c *Client) GetMe(ctx context.Context) (Me, error) {
	var resp struct {
		OK     bool `json:"ok"`
		Result Me   `json:"result"`
	}
	if err := c.get(ctx, "getMe", url.Values{}, &resp); err != nil {
		return Me{}, err
	}
	if !resp.OK {
		return Me{}, okFalse("getMe")
	}
	return resp.Result, nil
}

// DeleteWebhook makes sure long polling is possible: a bot with a webhook set
// gets 409 from getUpdates forever.
func (c *Client) DeleteWebhook(ctx context.Context) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "deleteWebhook", map[string]any{"drop_pending_updates": false}, &resp)
}

// GetUpdates long-polls. timeoutSeconds is Telegram's own poll duration; the
// HTTP deadline is derived from it, so a slow network cannot cut a poll short.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	values := url.Values{}
	values.Set("timeout", strconv.Itoa(timeoutSeconds))
	if len(c.allowed) > 0 {
		encoded, err := json.Marshal(c.allowed)
		if err != nil {
			return nil, err
		}
		values.Set("allowed_updates", string(encoded))
	}
	if offset > 0 {
		values.Set("offset", strconv.FormatInt(offset, 10))
	}
	var resp struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	reqTimeout := time.Duration(timeoutSeconds)*time.Second + 15*time.Second
	if err := c.getWithTimeout(ctx, "getUpdates", values, &resp, reqTimeout); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, okFalse("getUpdates")
	}
	return resp.Result, nil
}

// SetMyCommands publishes the default command list.
func (c *Client) SetMyCommands(ctx context.Context, commands []BotCommand) error {
	return c.SetMyCommandsForScope(ctx, commands, nil)
}

// SetMyCommandsForScope publishes one command list. language_code is a sibling
// of scope in the Bot API, not a field inside it, so it is lifted out here.
func (c *Client) SetMyCommandsForScope(ctx context.Context, commands []BotCommand, scope *BotCommandScope) error {
	req := map[string]any{"commands": commands}
	if scope != nil {
		req["scope"] = scope
		if scope.LanguageCode != "" {
			req["language_code"] = scope.LanguageCode
		}
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "setMyCommands", req, &resp)
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) (Message, error) {
	return c.sendMessage(ctx, map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": noLinkPreview,
	}, markup)
}

// SendReply sends an HTML message as a reply, optionally inside a forum topic.
func (c *Client) SendReply(ctx context.Context, chatID, replyTo int64, threadID int, text string, markup *InlineKeyboardMarkup) (Message, error) {
	req := map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": noLinkPreview,
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
	return c.sendMessage(ctx, req, markup)
}

func (c *Client) sendMessage(ctx context.Context, req map[string]any, markup *InlineKeyboardMarkup) (Message, error) {
	if markup != nil {
		req["reply_markup"] = markup
	}
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.post(ctx, "sendMessage", req, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse("sendMessage")
	}
	return resp.Result, nil
}

// EditMessageText replaces a message's text. A "message is not modified"
// answer is success: it means the rendering did not change.
func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string, markup *InlineKeyboardMarkup) error {
	req := map[string]any{
		"chat_id":              chatID,
		"message_id":           messageID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": noLinkPreview,
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

func (c *Client) DeleteMessage(ctx context.Context, chatID, messageID int64) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "deleteMessage", map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
	}, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("deleteMessage")
	}
	return nil
}

// AnswerCallbackQuery closes the spinner on a tapped inline button. text is
// optional and shows as a toast.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackID, text string) error {
	return c.answerCallbackQuery(ctx, callbackID, text, false)
}

// AnswerCallbackQueryAlert answers with a modal the user has to dismiss.
func (c *Client) AnswerCallbackQueryAlert(ctx context.Context, callbackID, text string) error {
	return c.answerCallbackQuery(ctx, callbackID, text, true)
}

func (c *Client) answerCallbackQuery(ctx context.Context, callbackID, text string, alert bool) error {
	req := map[string]any{"callback_query_id": callbackID}
	if text != "" {
		req["text"] = text
	}
	if alert {
		req["show_alert"] = true
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "answerCallbackQuery", req, &resp)
}

// SendChatAction shows "typing…" or "sending audio…" while work is in flight.
func (c *Client) SendChatAction(ctx context.Context, chatID int64, threadID int, action string) error {
	req := map[string]any{"chat_id": chatID, "action": action}
	if threadID > 0 {
		req["message_thread_id"] = threadID
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "sendChatAction", req, &resp)
}

func (c *Client) GetChatMember(ctx context.Context, chatID, userID int64) (ChatMember, error) {
	values := url.Values{}
	values.Set("chat_id", strconv.FormatInt(chatID, 10))
	values.Set("user_id", strconv.FormatInt(userID, 10))
	var resp struct {
		OK     bool       `json:"ok"`
		Result ChatMember `json:"result"`
	}
	if err := c.get(ctx, "getChatMember", values, &resp); err != nil {
		return ChatMember{}, err
	}
	if !resp.OK {
		return ChatMember{}, okFalse("getChatMember")
	}
	return resp.Result, nil
}

// LogOut releases the token from the current Bot API server so it can be used
// on another one. Telegram refuses to log back in to the cloud server for ten
// minutes afterwards, so this is only ever called deliberately — and never by
// [Client.Probe].
func (c *Client) LogOut(ctx context.Context) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "logOut", map[string]any{}, &resp)
}
