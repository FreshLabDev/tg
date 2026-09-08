// SPDX-License-Identifier: Apache-2.0
package tg

// The types here are the Telegram protocol, nothing more. What a bot does with
// an attachment — which one counts as speech, in what order to prefer them —
// is a product decision and lives in the bot.

type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	Username     string `json:"username"`
	LanguageCode string `json:"language_code"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type Voice struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

type VideoNote struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Length       int    `json:"length"`
	Duration     int    `json:"duration"`
	FileSize     int64  `json:"file_size"`
}

type Audio struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	FileName     string `json:"file_name"`
}

type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	FileName     string `json:"file_name"`
}

type Document struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	FileName     string `json:"file_name"`
}

type Photo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size"`
}

// File is a getFile result. FilePath is relative on Telegram's own server and
// absolute on a server started with --local; [Client.DownloadToFile] handles
// both.
type File struct {
	FileID   string `json:"file_id"`
	FilePath string `json:"file_path"`
	FileSize int64  `json:"file_size"`
}

type Message struct {
	MessageID int64 `json:"message_id"`
	// EphemeralMessageID is non-zero only for a Bot API 10.3 ephemeral message.
	EphemeralMessageID int64 `json:"ephemeral_message_id"`
	MessageThreadID    int   `json:"message_thread_id"`
	// From is absent on channel posts, so it is a pointer.
	From           *User      `json:"from"`
	ReceiverUser   *User      `json:"receiver_user"`
	Chat           Chat       `json:"chat"`
	Date           int64      `json:"date"`
	Text           string     `json:"text"`
	Caption        string     `json:"caption"`
	Voice          *Voice     `json:"voice"`
	VideoNote      *VideoNote `json:"video_note"`
	Audio          *Audio     `json:"audio"`
	Video          *Video     `json:"video"`
	Document       *Document  `json:"document"`
	Photo          []Photo    `json:"photo"`
	ReplyToMessage *Message   `json:"reply_to_message"`
}

type CallbackQuery struct {
	ID      string  `json:"id"`
	From    User    `json:"from"`
	Message Message `json:"message"`
	Data    string  `json:"data"`
}

type ChatMember struct {
	Status string `json:"status"`
	User   User   `json:"user"`
}

type ChatMemberUpdated struct {
	Chat          Chat       `json:"chat"`
	From          User       `json:"from"`
	NewChatMember ChatMember `json:"new_chat_member"`
}

type Update struct {
	UpdateID     int64              `json:"update_id"`
	Message      *Message           `json:"message"`
	Callback     *CallbackQuery     `json:"callback_query"`
	MyChatMember *ChatMemberUpdated `json:"my_chat_member"`
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
	Username  string `json:"username"`
}
