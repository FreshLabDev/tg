// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
)

// A draft is a message being written, streamed into a private chat while the
// bot works (Bot API 9.3, rich drafts 10.1, Stop 10.3). It is not a message:
// it has no id, cannot be edited or deleted, and lasts 30 seconds. A bot
// sends it again with the same draft id to change it, which Telegram
// animates, and sends the finished message as an ordinary one, which takes
// the draft's place. A message the bot sends to the chat clears the draft.

// DraftOptions are the parameters every draft send shares besides the chat,
// the draft id and the content.
type DraftOptions struct {
	// ThreadID streams into a topic of the private chat; zero is the chat.
	ThreadID int
	// CanStop shows the person a button that stops the drafts. A press
	// arrives as [Update.StoppedMessageGeneration] with the draft's id.
	CanStop bool
	// KeepOnStop leaves the draft on screen after Stop, for a short while or
	// until the bot sends a message. A draft that should stay is sent as a
	// message of its own.
	KeepOnStop bool
}

// SendMessageDraft streams a draft of a text message into a private chat.
// draftID must not be zero; sends with the same id change one draft. The
// text is HTML, like every other text this package sends, and an empty one
// shows Telegram's "Thinking…" placeholder.
func (c *Client) SendMessageDraft(ctx context.Context, chatID, draftID int64, text string, opts DraftOptions) error {
	req := map[string]any{"text": text}
	if text != "" {
		req["parse_mode"] = "HTML"
	}
	return c.sendDraft(ctx, "sendMessageDraft", chatID, draftID, req, opts)
}

// SendRichDraftHTML streams a draft of a rich message, as [Client.SendRichHTML]
// sends one. Telegram takes no upload and no file by URL in a draft.
func (c *Client) SendRichDraftHTML(ctx context.Context, chatID, draftID int64, body string, opts DraftOptions, rich ...RichOption) error {
	return c.sendDraft(ctx, "sendRichMessageDraft", chatID, draftID, map[string]any{"rich_message": richHTML(body, rich...)}, opts)
}

// SendRichDraftMarkdown is [Client.SendRichDraftHTML] for GFM, with the caveat
// of [Client.SendRichMarkdown]: only for content authored as markdown.
func (c *Client) SendRichDraftMarkdown(ctx context.Context, chatID, draftID int64, markdown string, opts DraftOptions, rich ...RichOption) error {
	return c.sendDraft(ctx, "sendRichMessageDraft", chatID, draftID, map[string]any{"rich_message": richMarkdown(markdown, rich...)}, opts)
}

func (c *Client) sendDraft(ctx context.Context, method string, chatID, draftID int64, req map[string]any, opts DraftOptions) error {
	if draftID == 0 {
		return fmt.Errorf("%s requires a draft id other than zero", method)
	}
	req["chat_id"] = chatID
	req["draft_id"] = draftID
	if opts.ThreadID > 0 {
		req["message_thread_id"] = opts.ThreadID
	}
	if opts.CanStop {
		req["can_stop"] = true
	}
	if opts.KeepOnStop {
		req["keep_on_stop"] = true
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

// MessageGenerationStopped is a press of Stop on a draft sent with
// [DraftOptions.CanStop]: the person wants no more of it. It names the chat,
// the topic and the draft, so a bot that streams several drafts at once
// knows which work to stop. It reaches a bot that polls with
// [WithAllowedUpdates] only when "stopped_message_generation" is named.
type MessageGenerationStopped struct {
	Chat Chat `json:"chat"`
	// ThreadID is the topic of the private chat; zero for the chat itself.
	ThreadID int   `json:"message_thread_id,omitempty"`
	DraftID  int64 `json:"draft_id"`
}
