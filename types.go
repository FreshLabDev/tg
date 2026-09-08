// SPDX-License-Identifier: Apache-2.0
package tg

import "encoding/json"

// The struct tags mirror Telegram's own required/optional split: a field the
// API always sends is always marshaled, an optional one only when set. That
// matters for a bot that stores a message as an audit record -- otherwise the
// record is mostly nulls this package invented.
//
// The types here are the Telegram protocol, nothing more. What a bot does with
// an attachment — which one counts as speech, in what order to prefer them —
// is a product decision and lives in the bot.

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Username string `json:"username,omitempty"`
}

type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type VideoNote struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Length       int    `json:"length"`
	Duration     int    `json:"duration"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileName     string `json:"file_name,omitempty"`
}

type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileName     string `json:"file_name,omitempty"`
}

type Document struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileName     string `json:"file_name,omitempty"`
}

type Photo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// File is a getFile result. FilePath is relative on Telegram's own server and
// absolute on a server started with --local; [Client.DownloadToFile] handles
// both.
type File struct {
	FileID string `json:"file_id"`
	// FileUniqueID is stable across the file_id rotations Telegram does, so it
	// is what a bot keyed on identity should store.
	FileUniqueID string `json:"file_unique_id"`
	FilePath     string `json:"file_path,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
}

type Message struct {
	MessageID int64 `json:"message_id"`
	// EphemeralMessageID is non-zero only for a Bot API 10.3 ephemeral message.
	EphemeralMessageID int64 `json:"ephemeral_message_id,omitempty"`
	MessageThreadID    int   `json:"message_thread_id,omitempty"`
	// From is absent on channel posts, so it is a pointer.
	From            *User           `json:"from,omitempty"`
	ReceiverUser    *User           `json:"receiver_user,omitempty"`
	Chat            Chat            `json:"chat"`
	Date            int64           `json:"date"`
	Text            string          `json:"text,omitempty"`
	Entities        []MessageEntity `json:"entities,omitempty"`
	Caption         string          `json:"caption,omitempty"`
	CaptionEntities []MessageEntity `json:"caption_entities,omitempty"`
	Voice           *Voice          `json:"voice,omitempty"`
	VideoNote       *VideoNote      `json:"video_note,omitempty"`
	Audio           *Audio          `json:"audio,omitempty"`
	Video           *Video          `json:"video,omitempty"`
	Document        *Document       `json:"document,omitempty"`
	Photo           []Photo         `json:"photo,omitempty"`
	ReplyToMessage  *Message        `json:"reply_to_message,omitempty"`

	// Raw is the message exactly as Telegram sent it. This package models the
	// fields the family uses and no more, so Raw is how a bot reaches a field
	// that is not modeled yet -- and how one that keeps an audit trail stores
	// what actually arrived rather than a re-encoding of this struct.
	Raw json.RawMessage `json:"-"`
}

func (m *Message) UnmarshalJSON(data []byte) error {
	// The alias sheds the method set, so decoding does not recurse.
	type message Message
	var decoded message
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Message(decoded)
	m.Raw = append(json.RawMessage(nil), data...)
	return nil
}

// MessageEntity is Telegram's own markup of a message's text: the bold run, the
// link, the code span. Offsets and lengths are in UTF-16 code units, not bytes
// and not runes, which is the detail that makes hand-rolling this painful.
type MessageEntity struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	URL           string `json:"url,omitempty"`
	User          *User  `json:"user,omitempty"`
	Language      string `json:"language,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

type CallbackQuery struct {
	ID   string `json:"id"`
	From User   `json:"from"`
	// Message is absent for a callback from an inline message, which none of
	// the family's bots send today. Reading Chat.ID off a zero value would
	// address chat 0, so a bot that adds inline mode must make this a pointer
	// first -- a change every caller has to see, which is why it is written
	// down rather than made now.
	Message Message `json:"message"`
	// ChatInstance identifies the message a callback came from even when
	// Message is absent.
	ChatInstance string `json:"chat_instance"`
	Data         string `json:"data,omitempty"`
}

type ChatMember struct {
	Status string `json:"status"`
	User   User   `json:"user"`
}

type ChatMemberUpdated struct {
	Chat Chat  `json:"chat"`
	From User  `json:"from"`
	Date int64 `json:"date"`
	// OldChatMember is what distinguishes being added to a group from being
	// promoted inside one, which is the whole reason a bot asks for
	// my_chat_member.
	OldChatMember ChatMember `json:"old_chat_member"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

type Update struct {
	UpdateID     int64              `json:"update_id"`
	Message      *Message           `json:"message,omitempty"`
	Callback     *CallbackQuery     `json:"callback_query,omitempty"`
	MyChatMember *ChatMemberUpdated `json:"my_chat_member,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
	// Style colors the button (Bot API 9.4+). Clients older than 2026-02-09
	// render it as a normal button, so it degrades gracefully.
	Style string `json:"style,omitempty"`
	// Disabled is the Bot API 10.3 action slot for an inert button. It cannot
	// be combined with CallbackData or URL.
	Disabled *DisabledButton `json:"disabled,omitempty"`
}

// DisabledButton is an empty Bot API 10.3 object: the presence of the field is
// what disables the button.
type DisabledButton struct{}

// Inline button color styles (Bot API 9.4).
const (
	StylePrimary = "primary" // proceed / save / connect
	StyleSuccess = "success" // terminal "done" action
	StyleDanger  = "danger"  // destructive
)

type BotCommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	IsEphemeral bool   `json:"is_ephemeral,omitempty"`
}

type BotCommandScope struct {
	Type string `json:"type"`
	// LanguageCode is carried here for convenience; setMyCommands takes it as a
	// sibling of scope, and the client lifts it out.
	LanguageCode string `json:"-"`
}

type Me struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username,omitempty"`
}
