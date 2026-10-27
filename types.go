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

type Animation struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
	MimeType     string `json:"mime_type,omitempty"`
	FileSize     int64  `json:"file_size,omitempty"`
	FileName     string `json:"file_name,omitempty"`
}

type Sticker struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	// Type is "regular", "mask" or "custom_emoji". It is the sticker's role and
	// says nothing about its format, which IsAnimated and IsVideo describe.
	Type       string `json:"type"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	IsAnimated bool   `json:"is_animated"`
	IsVideo    bool   `json:"is_video"`
	Emoji      string `json:"emoji,omitempty"`
	SetName    string `json:"set_name,omitempty"`
	FileSize   int64  `json:"file_size,omitempty"`
}

type Photo struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size,omitempty"`
}

// LivePhoto is a photo with a short video (Bot API 10.0). FileID is the video;
// Photo holds the sizes of the still frame. Telegram also fills
// [Message.Photo] with those sizes, so a bot that sorts attachments checks
// [Message.LivePhoto] before [Message.Photo], or it will take every live photo
// for a plain one.
type LivePhoto struct {
	Photo        []Photo `json:"photo,omitempty"`
	FileID       string  `json:"file_id"`
	FileUniqueID string  `json:"file_unique_id"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	Duration     int     `json:"duration"`
	MimeType     string  `json:"mime_type,omitempty"`
	FileSize     int64   `json:"file_size,omitempty"`
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
	// IsTopicMessage is true for a message that belongs to a topic. It is not
	// implied by MessageThreadID: Telegram also sets that on a reply chain in an
	// ordinary supergroup, where there is no topic to address, so a bot that
	// sends into a thread should check this first.
	IsTopicMessage bool `json:"is_topic_message,omitempty"`
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

	// ExternalReply describes the message being replied to when it is in
	// another chat or another topic. Such a reply arrives here and not in
	// ReplyToMessage, which Telegram sets only for a reply in the same chat and
	// thread, so a bot that reads the replied-to message looks at both.
	ExternalReply *ExternalReplyInfo `json:"external_reply,omitempty"`

	// MediaGroupID is shared by the messages of one album. Telegram delivers
	// every item of an album as its own update and does not say how many there
	// are or which one is last, so a bot that wants the whole album collects the
	// items by this id. The caption is usually on one of them. The id is unique
	// inside the chat only.
	MediaGroupID string `json:"media_group_id,omitempty"`

	// HasMediaSpoiler is true when the photo, video or animation is hidden under
	// a spoiler.
	HasMediaSpoiler bool `json:"has_media_spoiler,omitempty"`

	// Animation arrives together with Document, which Telegram fills for a GIF
	// as well for backward compatibility. A bot that sorts attachments must look
	// at Animation before Document or it will take every GIF for a file.
	Animation *Animation `json:"animation,omitempty"`
	Sticker   *Sticker   `json:"sticker,omitempty"`

	// LivePhoto arrives together with Photo, which Telegram fills with the sizes
	// of the still frame for backward compatibility. A bot that sorts attachments
	// must look at LivePhoto before Photo, as it does with Animation before
	// Document.
	LivePhoto *LivePhoto `json:"live_photo,omitempty"`

	// MigrateToChatID and MigrateFromChatID are service messages for a group
	// that became a supergroup under a new chat id: the first arrives in the old
	// group, the second in the new supergroup.
	MigrateToChatID   int64 `json:"migrate_to_chat_id,omitempty"`
	MigrateFromChatID int64 `json:"migrate_from_chat_id,omitempty"`

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

// MessageOrigin says where a message came from, flattened the way
// [ReactionType] is: Type names which of the other fields is set. A "user"
// origin sets SenderUser, a "hidden_user" one only SenderUserName, a "chat"
// one SenderChat, and a "channel" one Chat and MessageID.
type MessageOrigin struct {
	Type            string `json:"type"`
	Date            int64  `json:"date"`
	SenderUser      *User  `json:"sender_user,omitempty"`
	SenderUserName  string `json:"sender_user_name,omitempty"`
	SenderChat      *Chat  `json:"sender_chat,omitempty"`
	Chat            *Chat  `json:"chat,omitempty"`
	MessageID       int64  `json:"message_id,omitempty"`
	AuthorSignature string `json:"author_signature,omitempty"`
}

// Message origin kinds, as Telegram reports them in [MessageOrigin.Type].
const (
	OriginUser       = "user"
	OriginHiddenUser = "hidden_user"
	OriginChat       = "chat"
	OriginChannel    = "channel"
)

// ExternalReplyInfo is the message a reply points at when that message is in
// another chat or another topic. It has the attachments but no text and no
// caption: Telegram does not send them. Of the rest of the Bot API object only
// what the family reads is modeled; [Message.Raw] has everything that arrived.
//
// Like [Message], a photo of a live photo is in Photo as well as in LivePhoto,
// and a GIF is in Document as well as in Animation.
type ExternalReplyInfo struct {
	Origin MessageOrigin `json:"origin"`
	// Chat and MessageID are present only when the original chat is a
	// supergroup or a channel.
	Chat      *Chat      `json:"chat,omitempty"`
	MessageID int64      `json:"message_id,omitempty"`
	Animation *Animation `json:"animation,omitempty"`
	Audio     *Audio     `json:"audio,omitempty"`
	Document  *Document  `json:"document,omitempty"`
	LivePhoto *LivePhoto `json:"live_photo,omitempty"`
	Photo     []Photo    `json:"photo,omitempty"`
	Sticker   *Sticker   `json:"sticker,omitempty"`
	Video     *Video     `json:"video,omitempty"`
	VideoNote *VideoNote `json:"video_note,omitempty"`
	Voice     *Voice     `json:"voice,omitempty"`
	// HasMediaSpoiler is true when the media was hidden under a spoiler.
	HasMediaSpoiler bool `json:"has_media_spoiler,omitempty"`
}

type CallbackQuery struct {
	ID   string `json:"id"`
	From User   `json:"from"`
	// Message is absent for a callback from an inline message, which none of
	// the family's bots send today. Reading Chat.ID off a zero value would
	// address chat 0, so a bot that adds inline mode must make this a pointer
	// first -- a change every caller has to see, which is why it is written
	// down rather than made now.
	//
	// A message the bot can no longer access (deleted, or too old) arrives with
	// date 0 and nothing but Chat and MessageID, so a bot that edits the message
	// tests Message.Date == 0 before trusting any other field.
	Message Message `json:"message"`
	// ChatInstance identifies the message a callback came from even when
	// Message is absent.
	ChatInstance string `json:"chat_instance"`
	Data         string `json:"data,omitempty"`
}

type ChatMember struct {
	Status string `json:"status"`
	User   User   `json:"user"`
	// IsMember is sent only with status "restricted", where false means the user
	// is restricted but has left the chat. For every other status it is absent
	// and reads false, which means nothing.
	IsMember bool `json:"is_member,omitempty"`
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

// ReactionType is Telegram's union of an emoji, a custom emoji and a paid
// reaction, flattened: Type says which of the other fields is set, and a paid
// reaction sets neither.
type ReactionType struct {
	Type          string `json:"type"`
	Emoji         string `json:"emoji,omitempty"`
	CustomEmojiID string `json:"custom_emoji_id,omitempty"`
}

// Reaction kinds, as Telegram reports them in [ReactionType.Type].
const (
	ReactionEmoji       = "emoji"
	ReactionCustomEmoji = "custom_emoji"
	ReactionPaid        = "paid"
)

// ReactionCount is one reaction and how many times it is on a message.
type ReactionCount struct {
	Type       ReactionType `json:"type"`
	TotalCount int          `json:"total_count"`
}

// MessageReactionUpdated is one user changing their reactions on a message.
// OldReaction and NewReaction are the whole lists before and after, not the
// difference, so an added reaction is one that is in NewReaction and not in
// OldReaction. User is absent when the user is anonymous, and ActorChat is set
// instead. Telegram sends nothing for a reaction set by a bot.
type MessageReactionUpdated struct {
	Chat        Chat           `json:"chat"`
	MessageID   int64          `json:"message_id"`
	User        *User          `json:"user,omitempty"`
	ActorChat   *Chat          `json:"actor_chat,omitempty"`
	Date        int64          `json:"date"`
	OldReaction []ReactionType `json:"old_reaction"`
	NewReaction []ReactionType `json:"new_reaction"`
}

// MessageReactionCountUpdated carries the totals on a message whose reactions
// are anonymous. Telegram groups these updates and may deliver them minutes
// late.
type MessageReactionCountUpdated struct {
	Chat      Chat            `json:"chat"`
	MessageID int64           `json:"message_id"`
	Date      int64           `json:"date"`
	Reactions []ReactionCount `json:"reactions"`
}

type Update struct {
	UpdateID           int64               `json:"update_id"`
	Message            *Message            `json:"message,omitempty"`
	Callback           *CallbackQuery      `json:"callback_query,omitempty"`
	MyChatMember       *ChatMemberUpdated  `json:"my_chat_member,omitempty"`
	InlineQuery        *InlineQuery        `json:"inline_query,omitempty"`
	ChosenInlineResult *ChosenInlineResult `json:"chosen_inline_result,omitempty"`

	// EditedMessage is the new version of a message that was edited. An edit does
	// not arrive as Message.
	EditedMessage *Message `json:"edited_message,omitempty"`

	// MessageReaction and MessageReactionCount reach only a bot that is an
	// administrator of the chat and has named them in [WithAllowedUpdates].
	MessageReaction      *MessageReactionUpdated      `json:"message_reaction,omitempty"`
	MessageReactionCount *MessageReactionCountUpdated `json:"message_reaction_count,omitempty"`
}

// LinkPreviewOptions controls Telegram's own preview of a link in a message.
// Everything this package sends disables it by default: a bot's message is
// usually the content, and a preview of a URL inside it is noise that also
// changes the message's height under the reader.
type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
	// URL previews a different link than the first one in the text.
	URL              string `json:"url,omitempty"`
	PreferSmallMedia bool   `json:"prefer_small_media,omitempty"`
	PreferLargeMedia bool   `json:"prefer_large_media,omitempty"`
	ShowAboveText    bool   `json:"show_above_text,omitempty"`
}

// Chat actions, the "…is typing" line under a chat title. Telegram clears one
// after five seconds, so anything longer has to repeat it.
const (
	ChatActionTyping          = "typing"
	ChatActionUploadPhoto     = "upload_photo"
	ChatActionRecordVideo     = "record_video"
	ChatActionUploadVideo     = "upload_video"
	ChatActionRecordVoice     = "record_voice"
	ChatActionUploadVoice     = "upload_voice"
	ChatActionUploadDocument  = "upload_document"
	ChatActionChooseSticker   = "choose_sticker"
	ChatActionFindLocation    = "find_location"
	ChatActionRecordVideoNote = "record_video_note"
	ChatActionUploadVideoNote = "upload_video_note"
)

// Chat types, as Telegram reports them in [Chat.Type]. A bot that behaves
// differently in a group than in a DM compares against these rather than
// spelling the strings out at every branch.
const (
	ChatPrivate    = "private"
	ChatGroup      = "group"
	ChatSupergroup = "supergroup"
	ChatChannel    = "channel"
)

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
	// SwitchInlineQueryCurrentChat opens inline mode in the chat the button is
	// in, with this string already typed. It is a pointer because the empty
	// string is the useful value -- "open inline search with nothing typed" --
	// and omitempty would drop exactly that.
	SwitchInlineQueryCurrentChat *string `json:"switch_inline_query_current_chat,omitempty"`
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
