// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"
)

// noLinkPreview replaces the removed disable_web_page_preview parameter.
var noLinkPreview = &LinkPreviewOptions{IsDisabled: true}

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

// SendPlainText sends text with no parse mode at all. It is the delivery of
// last resort: HTML that Telegram rejects -- malformed, or using a tag a
// newer client stopped accepting -- fails the whole send, and a notification
// that arrives unformatted beats one that does not arrive.
func (c *Client) SendPlainText(ctx context.Context, chatID int64, text string) (Message, error) {
	return c.sendMessage(ctx, map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"link_preview_options": noLinkPreview,
	}, nil)
}

// SendTextWithPreview sends HTML text and lets Telegram render its own preview
// of a link inside it. Every other send here suppresses the preview, because a
// bot's own message is normally the content; this exists for a bot relaying
// text whose author decided otherwise. A nil preview disables it, matching the
// rest of the package.
func (c *Client) SendTextWithPreview(ctx context.Context, chatID int64, threadID int, text string, preview *LinkPreviewOptions, markup *InlineKeyboardMarkup) (Message, error) {
	if preview == nil {
		preview = noLinkPreview
	}
	req := map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"parse_mode":           "HTML",
		"link_preview_options": preview,
	}
	if threadID > 0 {
		req["message_thread_id"] = threadID
	}
	return c.sendMessage(ctx, req, markup)
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

// maxDeleteMessages is how many messages one deleteMessages call takes.
const maxDeleteMessages = 100

// DeleteMessages deletes several messages of one chat in a single call. Ids
// that Telegram cannot find are skipped, not reported, so a nil error does not
// say every message is gone. It takes 1 to 100 ids and refuses anything else
// before sending; a bot with more splits them. The limits on which messages can
// be deleted are those of [Client.DeleteMessage].
func (c *Client) DeleteMessages(ctx context.Context, chatID int64, messageIDs []int64) error {
	if len(messageIDs) == 0 {
		return fmt.Errorf("deleteMessages requires at least one message id")
	}
	if len(messageIDs) > maxDeleteMessages {
		return fmt.Errorf("deleteMessages takes at most %d message ids, got %d", maxDeleteMessages, len(messageIDs))
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "deleteMessages", map[string]any{
		"chat_id":     chatID,
		"message_ids": messageIDs,
	}, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("deleteMessages")
	}
	return nil
}

// SetMessageReaction sets the bot's own reaction on a message, replacing the one
// it set before, and an empty list removes it. A bot can have one reaction per
// message.
//
// Telegram takes only the emoji in the list under ReactionTypeEmoji in the Bot
// API reference. 👀 is in it and ✅ is not, so a bot that wants a "done" mark
// picks another, such as 👌 or 🔥. A custom emoji works only if it is already on
// the message or the chat's administrators allow it, and a paid reaction is
// refused for a bot. The chat can also restrict which reactions it offers.
//
// For a message in an album the reaction lands on the first message of the
// album that has not been deleted, whichever id is passed.
//
// To remove somebody else's reaction see [Client.DeleteMessageReaction].
func (c *Client) SetMessageReaction(ctx context.Context, chatID, messageID int64, reactions []ReactionType) error {
	req := map[string]any{"chat_id": chatID, "message_id": messageID}
	if len(reactions) > 0 {
		req["reaction"] = reactions
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "setMessageReaction", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("setMessageReaction")
	}
	return nil
}

// ReactionActor names whose reaction [Client.DeleteMessageReaction] removes:
// exactly one of UserID and ActorChatID, the two ways a reaction is attributed.
// They are the same two values a [MessageReactionUpdated] update carries, as
// User.ID and ActorChat.ID.
type ReactionActor struct {
	UserID      int64
	ActorChatID int64
}

// DeleteMessageReaction removes the reaction a user, or a chat acting as one,
// put on a message. It is a moderation method (Bot API 10.0) and not the way to
// take back the bot's own reaction, which [Client.SetMessageReaction] does with
// an empty list. It works in groups and supergroups only, and the bot needs the
// administrator right to delete messages there.
func (c *Client) DeleteMessageReaction(ctx context.Context, chatID, messageID int64, actor ReactionActor) error {
	req := map[string]any{"chat_id": chatID, "message_id": messageID}
	switch {
	case actor.UserID != 0 && actor.ActorChatID != 0:
		return fmt.Errorf("deleteMessageReaction takes a user or a chat, not both")
	case actor.UserID != 0:
		req["user_id"] = actor.UserID
	case actor.ActorChatID != 0:
		req["actor_chat_id"] = actor.ActorChatID
	default:
		return fmt.Errorf("deleteMessageReaction requires the user or the chat whose reaction to remove")
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "deleteMessageReaction", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("deleteMessageReaction")
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

// AnswerCallbackQueryURL answers a tapped button by opening a link, which for
// a t.me deep link means the client follows it without an intermediate
// message. Telegram only honors a URL that points at the bot itself or at a
// game it owns; anything else is ignored, and the tap looks like it did
// nothing.
func (c *Client) AnswerCallbackQueryURL(ctx context.Context, callbackID, url string) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	return c.post(ctx, "answerCallbackQuery", map[string]any{
		"callback_query_id": callbackID,
		"url":               url,
	}, &resp)
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

// SetChatMenuButton changes the menu button in a private chat with the bot, or
// the bot's default button when chatID is 0. A chat's own button wins over the
// default, and [MenuDefault] removes the chat's own.
//
// A web_app button needs both Text and WebApp.URL, and an unknown Type is not a
// button at all; both are refused before sending. An empty Type is sent as
// [MenuDefault].
func (c *Client) SetChatMenuButton(ctx context.Context, chatID int64, button MenuButton) error {
	menu := map[string]any{"type": button.Type}
	switch button.Type {
	case "", MenuDefault:
		menu["type"] = MenuDefault
	case MenuCommands:
	case MenuWebApp:
		if button.Text == "" || button.WebApp == nil || button.WebApp.URL == "" {
			return fmt.Errorf("a web_app menu button requires Text and WebApp.URL")
		}
		menu["text"] = button.Text
		menu["web_app"] = button.WebApp
	default:
		return fmt.Errorf("unknown menu button type %q", button.Type)
	}
	req := map[string]any{"menu_button": menu}
	if chatID != 0 {
		req["chat_id"] = chatID
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, "setChatMenuButton", req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse("setChatMenuButton")
	}
	return nil
}

// SetMyDescription sets the text shown in an empty chat with the bot, up to 512
// characters. languageCode is a two-letter ISO 639-1 code for the users it is
// shown to; empty sets the text for every user who has no description of their
// own language. An empty description is sent, not left out: it removes the
// description for that language.
func (c *Client) SetMyDescription(ctx context.Context, description, languageCode string) error {
	return c.setBotText(ctx, "setMyDescription", "description", description, languageCode)
}

// SetMyShortDescription sets the text on the bot's profile page, which is sent
// with the link when someone shares the bot, up to 120 characters. languageCode
// and an empty text work as for [Client.SetMyDescription].
func (c *Client) SetMyShortDescription(ctx context.Context, shortDescription, languageCode string) error {
	return c.setBotText(ctx, "setMyShortDescription", "short_description", shortDescription, languageCode)
}

func (c *Client) setBotText(ctx context.Context, method, field, text, languageCode string) error {
	req := map[string]any{field: text}
	if languageCode != "" {
		req["language_code"] = languageCode
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, method, req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse(method)
	}
	return nil
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
