// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
)

// A prepared inline message is a result the bot saves for one person, so
// that a Mini App can hand its id to Telegram.WebApp.shareMessage and the
// person picks the chat it goes to (Bot API 8.0). The chat need not have the
// bot in it. The message goes as an inline result does, "via" the bot, and
// takes any of the result types: a file Telegram already has by its file_id,
// or one it fetches from a URL.

// PreparedChats are the kinds of chat a prepared message may be sent to.
// Telegram needs at least one.
type PreparedChats struct {
	Users, Bots, Groups, Channels bool
}

// AnyChat lets a prepared message go to every kind of chat.
var AnyChat = PreparedChats{Users: true, Bots: true, Groups: true, Channels: true}

// PreparedInlineMessage is what savePreparedInlineMessage answers: the id a
// Mini App passes to shareMessage, and when it stops working, in Unix time.
type PreparedInlineMessage struct {
	ID             string `json:"id"`
	ExpirationDate int64  `json:"expiration_date"`
}

// SavePreparedInlineMessage saves result for userID to send from a Mini App.
// A result Telegram would refuse, or no kind of chat to send to, is refused
// before anything is sent.
func (c *Client) SavePreparedInlineMessage(ctx context.Context, userID int64, result InlineQueryResult, chats PreparedChats) (PreparedInlineMessage, error) {
	switch {
	case userID == 0:
		return PreparedInlineMessage{}, fmt.Errorf("savePreparedInlineMessage requires a user id")
	case result == nil:
		return PreparedInlineMessage{}, fmt.Errorf("savePreparedInlineMessage requires a result")
	case chats == PreparedChats{}:
		return PreparedInlineMessage{}, fmt.Errorf("savePreparedInlineMessage requires at least one kind of chat")
	}
	req := map[string]any{"user_id": userID, "result": result}
	for name, allowed := range map[string]bool{
		"allow_user_chats": chats.Users, "allow_bot_chats": chats.Bots,
		"allow_group_chats": chats.Groups, "allow_channel_chats": chats.Channels,
	} {
		if allowed {
			req[name] = true
		}
	}
	var resp struct {
		OK     bool                  `json:"ok"`
		Result PreparedInlineMessage `json:"result"`
	}
	if err := c.post(ctx, "savePreparedInlineMessage", req, &resp); err != nil {
		return PreparedInlineMessage{}, err
	}
	if !resp.OK {
		return PreparedInlineMessage{}, okFalse("savePreparedInlineMessage")
	}
	if resp.Result.ID == "" {
		return PreparedInlineMessage{}, fmt.Errorf("telegram savePreparedInlineMessage returned no id")
	}
	return resp.Result, nil
}
