// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
)

// Guest mode (Bot API 10.0) lets a person summon the bot into a chat it is
// not a member of, by mentioning it or replying to its message. The bot gets
// the summoning message as [Update.GuestMessage], with the message it replied
// to, and answers it once with an inline result. The answer is an inline
// message: the bot changes it later by its inline_message_id, with the
// EditInlineMessage methods, and cannot send anything else to that chat.

// SentGuestMessage is what answerGuestQuery answers: the inline message the
// answer became.
type SentGuestMessage struct {
	InlineMessageID string `json:"inline_message_id"`
}

// AnswerGuestQuery answers a guest message with result. A query is answered
// once; a missing id or result is refused before anything is sent.
func (c *Client) AnswerGuestQuery(ctx context.Context, guestQueryID string, result InlineQueryResult) (SentGuestMessage, error) {
	switch {
	case guestQueryID == "":
		return SentGuestMessage{}, fmt.Errorf("answerGuestQuery requires a guest_query_id")
	case result == nil:
		return SentGuestMessage{}, fmt.Errorf("answerGuestQuery requires a result")
	}
	var resp struct {
		OK     bool             `json:"ok"`
		Result SentGuestMessage `json:"result"`
	}
	req := map[string]any{"guest_query_id": guestQueryID, "result": result}
	if err := c.post(ctx, "answerGuestQuery", req, &resp); err != nil {
		return SentGuestMessage{}, err
	}
	if !resp.OK {
		return SentGuestMessage{}, okFalse("answerGuestQuery")
	}
	if resp.Result.InlineMessageID == "" {
		return SentGuestMessage{}, fmt.Errorf("telegram answerGuestQuery returned no inline_message_id")
	}
	return resp.Result, nil
}
