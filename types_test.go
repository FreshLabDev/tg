// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
)

// The samples below are shaped like what the Bot API sends, including fields
// this package does not model: decoding must ignore those, and Message.Raw must
// keep them.

const (
	sampleUser = `{"id":5550001,"is_bot":false,"first_name":"Ann","username":"ann","language_code":"en"}`
	sampleBot  = `{"id":777000111,"is_bot":true,"first_name":"Quoto","username":"quoto_bot"}`
	// An id past 32 bits, as every supergroup has.
	sampleSupergroup = `{"id":-1001234567890,"title":"Quotes","type":"supergroup"}`
	sampleOldGroup   = `{"id":-4012345678,"title":"Quotes","type":"group"}`
)

func TestUpdateDecodesEveryKindTheFamilyAsksFor(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, u Update)
	}{
		{
			name: "edited message",
			input: `{"update_id":900000001,"edited_message":{"message_id":412,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,` +
				`"date":1760000000,"edit_date":1760000090,` +
				`"reply_to_message":{"message_id":410,"from":` + sampleBot + `,"chat":` + sampleSupergroup + `,"date":1759999900,"text":"original"},` +
				`"text":"see https://example.test","entities":[{"offset":4,"length":20,"type":"url"}]}}`,
			check: func(t *testing.T, u Update) {
				m := u.EditedMessage
				if m == nil {
					t.Fatal("edited_message was dropped")
				}
				if u.Message != nil {
					t.Fatal("an edit must not appear as a new message")
				}
				if m.MessageID != 412 || m.Chat.ID != -1001234567890 || m.Text != "see https://example.test" {
					t.Fatalf("edited message = %+v", m)
				}
				if m.From == nil || m.From.ID != 5550001 {
					t.Fatalf("from = %+v", m.From)
				}
				if len(m.Entities) != 1 || m.Entities[0].Type != "url" || m.Entities[0].Length != 20 {
					t.Fatalf("entities = %+v", m.Entities)
				}
				if m.ReplyToMessage == nil || m.ReplyToMessage.MessageID != 410 {
					t.Fatalf("reply_to_message = %+v", m.ReplyToMessage)
				}
				if len(m.Raw) == 0 {
					t.Fatal("an edited message keeps Raw like any other")
				}
			},
		},
		{
			name: "reaction added",
			input: `{"update_id":900000002,"message_reaction":{"chat":` + sampleSupergroup + `,"message_id":412,"user":` + sampleUser + `,` +
				`"date":1760000100,"old_reaction":[],"new_reaction":[{"type":"emoji","emoji":"👍"}]}}`,
			check: func(t *testing.T, u Update) {
				r := u.MessageReaction
				if r == nil {
					t.Fatal("message_reaction was dropped")
				}
				if r.Chat.ID != -1001234567890 || r.MessageID != 412 || r.Date != 1760000100 {
					t.Fatalf("reaction = %+v", r)
				}
				if r.User == nil || r.User.ID != 5550001 || r.ActorChat != nil {
					t.Fatalf("user = %+v, actor_chat = %+v", r.User, r.ActorChat)
				}
				if len(r.OldReaction) != 0 {
					t.Fatalf("old_reaction = %+v, want none", r.OldReaction)
				}
				want := []ReactionType{{Type: ReactionEmoji, Emoji: "👍"}}
				if !reflect.DeepEqual(r.NewReaction, want) {
					t.Fatalf("new_reaction = %+v, want %+v", r.NewReaction, want)
				}
			},
		},
		{
			name: "custom emoji reaction",
			input: `{"update_id":900000003,"message_reaction":{"chat":` + sampleSupergroup + `,"message_id":412,"user":` + sampleUser + `,` +
				`"date":1760000110,"old_reaction":[{"type":"emoji","emoji":"👍"}],` +
				`"new_reaction":[{"type":"emoji","emoji":"👍"},{"type":"custom_emoji","custom_emoji_id":"5368324170671202286"}]}}`,
			check: func(t *testing.T, u Update) {
				r := u.MessageReaction
				if r == nil {
					t.Fatal("message_reaction was dropped")
				}
				want := []ReactionType{
					{Type: ReactionEmoji, Emoji: "👍"},
					{Type: ReactionCustomEmoji, CustomEmojiID: "5368324170671202286"},
				}
				if !reflect.DeepEqual(r.NewReaction, want) {
					t.Fatalf("new_reaction = %+v, want %+v", r.NewReaction, want)
				}
				if len(r.OldReaction) != 1 || r.OldReaction[0].Emoji != "👍" {
					t.Fatalf("old_reaction = %+v", r.OldReaction)
				}
				if r.NewReaction[1].Emoji != "" {
					t.Fatal("a custom emoji reaction carries no emoji")
				}
			},
		},
		{
			name: "paid reaction",
			input: `{"update_id":900000004,"message_reaction":{"chat":` + sampleSupergroup + `,"message_id":413,"user":` + sampleUser + `,` +
				`"date":1760000120,"old_reaction":[],"new_reaction":[{"type":"paid"}]}}`,
			check: func(t *testing.T, u Update) {
				r := u.MessageReaction
				if r == nil {
					t.Fatal("message_reaction was dropped")
				}
				want := []ReactionType{{Type: ReactionPaid}}
				if !reflect.DeepEqual(r.NewReaction, want) {
					t.Fatalf("new_reaction = %+v, want %+v", r.NewReaction, want)
				}
			},
		},
		{
			name: "anonymous reaction names the chat instead of the user",
			input: `{"update_id":900000005,"message_reaction":{"chat":` + sampleSupergroup + `,"message_id":414,` +
				`"actor_chat":` + sampleSupergroup + `,"date":1760000130,"old_reaction":[{"type":"emoji","emoji":"🔥"}],"new_reaction":[]}}`,
			check: func(t *testing.T, u Update) {
				r := u.MessageReaction
				if r == nil {
					t.Fatal("message_reaction was dropped")
				}
				if r.User != nil {
					t.Fatalf("user = %+v, want none for an anonymous reaction", r.User)
				}
				if r.ActorChat == nil || r.ActorChat.ID != -1001234567890 {
					t.Fatalf("actor_chat = %+v", r.ActorChat)
				}
				if len(r.NewReaction) != 0 || len(r.OldReaction) != 1 {
					t.Fatalf("a removed reaction: old = %+v, new = %+v", r.OldReaction, r.NewReaction)
				}
			},
		},
		{
			name: "reaction counts",
			input: `{"update_id":900000006,"message_reaction_count":{"chat":` + sampleSupergroup + `,"message_id":412,"date":1760000140,` +
				`"reactions":[{"type":{"type":"emoji","emoji":"🔥"},"total_count":3},` +
				`{"type":{"type":"custom_emoji","custom_emoji_id":"5368324170671202286"},"total_count":1},` +
				`{"type":{"type":"paid"},"total_count":25}]}}`,
			check: func(t *testing.T, u Update) {
				c := u.MessageReactionCount
				if c == nil {
					t.Fatal("message_reaction_count was dropped")
				}
				if u.MessageReaction != nil {
					t.Fatal("a count update is not a reaction update")
				}
				if c.Chat.ID != -1001234567890 || c.MessageID != 412 || c.Date != 1760000140 {
					t.Fatalf("count update = %+v", c)
				}
				want := []ReactionCount{
					{Type: ReactionType{Type: ReactionEmoji, Emoji: "🔥"}, TotalCount: 3},
					{Type: ReactionType{Type: ReactionCustomEmoji, CustomEmojiID: "5368324170671202286"}, TotalCount: 1},
					{Type: ReactionType{Type: ReactionPaid}, TotalCount: 25},
				}
				if !reflect.DeepEqual(c.Reactions, want) {
					t.Fatalf("reactions = %+v, want %+v", c.Reactions, want)
				}
			},
		},
		{
			name: "restricted member inside my_chat_member",
			input: `{"update_id":900000007,"my_chat_member":{"chat":` + sampleSupergroup + `,"from":` + sampleUser + `,"date":1760000150,` +
				`"old_chat_member":{"user":` + sampleBot + `,"status":"restricted","is_member":true,"can_send_messages":false,"can_send_audios":true,` +
				`"can_send_documents":true,"can_send_photos":true,"can_send_videos":true,"can_send_video_notes":true,"can_send_voice_notes":true,` +
				`"can_send_polls":true,"can_send_other_messages":true,"can_add_web_page_previews":true,"can_change_info":false,` +
				`"can_invite_users":true,"can_pin_messages":false,"can_manage_topics":false,"until_date":0},` +
				`"new_chat_member":{"user":` + sampleBot + `,"status":"administrator","can_be_edited":false,"is_anonymous":false,` +
				`"can_manage_chat":true,"can_delete_messages":true}}}`,
			check: func(t *testing.T, u Update) {
				m := u.MyChatMember
				if m == nil {
					t.Fatal("my_chat_member was dropped")
				}
				if m.OldChatMember.Status != "restricted" || !m.OldChatMember.IsMember {
					t.Fatalf("old member = %+v, want a restricted member who is still in the chat", m.OldChatMember)
				}
				if m.NewChatMember.Status != "administrator" || m.NewChatMember.IsMember {
					t.Fatalf("new member = %+v", m.NewChatMember)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var u Update
			if err := json.Unmarshal([]byte(tt.input), &u); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if u.UpdateID == 0 {
				t.Fatal("update_id was not decoded")
			}
			tt.check(t, u)
		})
	}
}

func TestMessageDecodesTheFieldsAQuoteBotReads(t *testing.T) {
	const thumb = `{"file_id":"AAMCAgADGQEAAg","file_unique_id":"AQADBw","file_size":3512,"width":128,"height":128}`
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, m Message)
	}{
		{
			name: "sticker",
			input: `{"message_id":501,"from":` + sampleUser + `,"chat":{"id":5550001,"first_name":"Ann","username":"ann","type":"private"},"date":1760000200,` +
				`"sticker":{"width":512,"height":512,"emoji":"😂","set_name":"HotCherry","is_animated":false,"is_video":true,"type":"regular",` +
				`"thumbnail":` + thumb + `,"thumb":` + thumb + `,"file_id":"CAACAgIAAxkBAAIB9W","file_unique_id":"AgADlQ0","file_size":24818}}`,
			check: func(t *testing.T, m Message) {
				want := Sticker{
					FileID: "CAACAgIAAxkBAAIB9W", FileUniqueID: "AgADlQ0", Type: "regular",
					Width: 512, Height: 512, IsVideo: true, Emoji: "😂", SetName: "HotCherry", FileSize: 24818,
				}
				if m.Sticker == nil || *m.Sticker != want {
					t.Fatalf("sticker = %+v, want %+v", m.Sticker, want)
				}
				if m.Animation != nil || m.Document != nil {
					t.Fatalf("a sticker is neither an animation nor a document: %+v %+v", m.Animation, m.Document)
				}
			},
		},
		{
			name: "animated sticker",
			input: `{"message_id":502,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,"date":1760000210,` +
				`"sticker":{"width":512,"height":512,"emoji":"🎉","set_name":"Confetti","is_animated":true,"is_video":false,"type":"regular",` +
				`"file_id":"CAACAgIAAxkBAAIB9X","file_unique_id":"AgADmQ0"}}`,
			check: func(t *testing.T, m Message) {
				if m.Sticker == nil || !m.Sticker.IsAnimated || m.Sticker.IsVideo {
					t.Fatalf("sticker = %+v, want an animated, non-video sticker", m.Sticker)
				}
				if m.Sticker.FileSize != 0 {
					t.Fatalf("file_size = %d, want 0 when Telegram omits it", m.Sticker.FileSize)
				}
			},
		},
		{
			name: "animation arrives together with document",
			input: `{"message_id":503,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,"date":1760000220,` +
				`"animation":{"file_name":"cat.mp4","mime_type":"video/mp4","duration":3,"width":480,"height":270,"thumbnail":` + thumb + `,"thumb":` + thumb + `,` +
				`"file_id":"CgACAgIAAxkBAAIC","file_unique_id":"AgADoAM","file_size":218840},` +
				`"document":{"file_name":"cat.mp4","mime_type":"video/mp4","thumbnail":` + thumb + `,"thumb":` + thumb + `,` +
				`"file_id":"CgACAgIAAxkBAAIC","file_unique_id":"AgADoAM","file_size":218840},"caption":"him"}`,
			check: func(t *testing.T, m Message) {
				want := Animation{
					FileID: "CgACAgIAAxkBAAIC", FileUniqueID: "AgADoAM", Width: 480, Height: 270, Duration: 3,
					MimeType: "video/mp4", FileName: "cat.mp4", FileSize: 218840,
				}
				if m.Animation == nil || *m.Animation != want {
					t.Fatalf("animation = %+v, want %+v", m.Animation, want)
				}
				if m.Document == nil || m.Document.FileUniqueID != m.Animation.FileUniqueID {
					t.Fatalf("document = %+v, want the same file as the animation", m.Document)
				}
				if m.Caption != "him" {
					t.Fatalf("caption = %q", m.Caption)
				}
			},
		},
		{
			name:  "group migrated to a supergroup",
			input: `{"message_id":9,"from":` + sampleUser + `,"chat":` + sampleOldGroup + `,"date":1760000230,"migrate_to_chat_id":-1001234567890}`,
			check: func(t *testing.T, m Message) {
				if m.MigrateToChatID != -1001234567890 {
					t.Fatalf("migrate_to_chat_id = %d, want it intact past 32 bits", m.MigrateToChatID)
				}
				if m.MigrateFromChatID != 0 {
					t.Fatalf("migrate_from_chat_id = %d", m.MigrateFromChatID)
				}
			},
		},
		{
			name:  "supergroup migrated from a group",
			input: `{"message_id":1,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,"date":1760000231,"migrate_from_chat_id":-4012345678}`,
			check: func(t *testing.T, m Message) {
				if m.MigrateFromChatID != -4012345678 {
					t.Fatalf("migrate_from_chat_id = %d", m.MigrateFromChatID)
				}
				if m.MigrateToChatID != 0 {
					t.Fatalf("migrate_to_chat_id = %d", m.MigrateToChatID)
				}
			},
		},
		{
			name: "message in a forum topic",
			input: `{"message_id":88,"message_thread_id":4,"from":` + sampleUser + `,"chat":{"id":-1001234567890,"title":"Quotes","is_forum":true,"type":"supergroup"},` +
				`"date":1760000240,"is_topic_message":true,` +
				`"reply_to_message":{"message_id":4,"from":` + sampleUser + `,"chat":{"id":-1001234567890,"title":"Quotes","is_forum":true,"type":"supergroup"},` +
				`"date":1760000000,"message_thread_id":4,"forum_topic_created":{"name":"Daily","icon_color":7322096},"is_topic_message":true},` +
				`"text":"hello"}`,
			check: func(t *testing.T, m Message) {
				if !m.IsTopicMessage || m.MessageThreadID != 4 {
					t.Fatalf("topic message = %v, thread = %d", m.IsTopicMessage, m.MessageThreadID)
				}
				if m.ReplyToMessage == nil || !m.ReplyToMessage.IsTopicMessage {
					t.Fatalf("reply_to_message = %+v, want the topic's first message to decode the flag too", m.ReplyToMessage)
				}
			},
		},
		{
			name: "reply chain in an ordinary supergroup has a thread id but no topic",
			input: `{"message_id":90,"message_thread_id":410,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,"date":1760000250,` +
				`"reply_to_message":{"message_id":410,"from":` + sampleBot + `,"chat":` + sampleSupergroup + `,"date":1759999900,"text":"original"},"text":"agreed"}`,
			check: func(t *testing.T, m Message) {
				if m.MessageThreadID != 410 {
					t.Fatalf("message_thread_id = %d", m.MessageThreadID)
				}
				if m.IsTopicMessage {
					t.Fatal("without is_topic_message there is no topic to send into")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Message
			if err := json.Unmarshal([]byte(tt.input), &m); err != nil {
				t.Fatalf("decode: %v", err)
			}
			tt.check(t, m)
		})
	}
}

func TestChatMemberReportsMembershipOfARestrictedUser(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantStatus string
		wantMember bool
	}{
		{
			name:       "restricted and still in the chat",
			input:      `{"user":` + sampleUser + `,"status":"restricted","is_member":true,"can_send_messages":false,"until_date":0}`,
			wantStatus: "restricted", wantMember: true,
		},
		{
			name:       "restricted and gone",
			input:      `{"user":` + sampleUser + `,"status":"restricted","is_member":false,"can_send_messages":false,"until_date":0}`,
			wantStatus: "restricted", wantMember: false,
		},
		{
			name:       "an ordinary member has no such field",
			input:      `{"user":` + sampleUser + `,"status":"member"}`,
			wantStatus: "member", wantMember: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var member ChatMember
			if err := json.Unmarshal([]byte(tt.input), &member); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if member.Status != tt.wantStatus || member.IsMember != tt.wantMember {
				t.Fatalf("member = %+v, want status %q and is_member %v", member, tt.wantStatus, tt.wantMember)
			}
		})
	}
}

func TestGetChatMemberCarriesIsMember(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"user":` + sampleUser + `,"status":"restricted","is_member":true,"can_send_messages":false,"until_date":0}}`))
	})

	member, err := c.GetChatMember(context.Background(), -1001234567890, 5550001)
	if err != nil {
		t.Fatalf("GetChatMember: %v", err)
	}
	if member.Status != "restricted" || !member.IsMember {
		t.Fatalf("member = %+v", member)
	}
}

// Message has its own UnmarshalJSON so it can keep Raw. A field added to the
// struct must still come through it, in the update itself and in a message
// nested inside another.
func TestNewFieldsDecodeThroughMessageUnmarshalJSON(t *testing.T) {
	const reply = `{"message_id":600,"from":` + sampleBot + `,"chat":` + sampleSupergroup + `,"date":1760000300,` +
		`"sticker":{"width":512,"height":512,"emoji":"😂","is_animated":false,"is_video":false,"type":"regular","file_id":"CAAC","file_unique_id":"AgAD"}}`
	const message = `{"message_id":601,"message_thread_id":4,"from":` + sampleUser + `,"chat":` + sampleSupergroup + `,"date":1760000310,"edit_date":1760000320,` +
		`"is_topic_message":true,` +
		`"animation":{"duration":3,"width":480,"height":270,"file_id":"CgAC","file_unique_id":"AgAD2"},` +
		`"sticker":{"width":512,"height":512,"is_animated":true,"is_video":false,"type":"mask","file_id":"CAAC2","file_unique_id":"AgAD3"},` +
		`"migrate_to_chat_id":-1001234567890,"migrate_from_chat_id":-4012345678,` +
		`"reply_to_message":` + reply + `}`

	check := func(t *testing.T, name string, m *Message) {
		t.Helper()
		if m == nil {
			t.Fatalf("%s: message was dropped", name)
		}
		if m.Animation == nil || m.Animation.FileID != "CgAC" {
			t.Errorf("%s: animation = %+v", name, m.Animation)
		}
		if m.Sticker == nil || m.Sticker.Type != "mask" || !m.Sticker.IsAnimated {
			t.Errorf("%s: sticker = %+v", name, m.Sticker)
		}
		if m.MigrateToChatID != -1001234567890 || m.MigrateFromChatID != -4012345678 {
			t.Errorf("%s: migrate to %d, from %d", name, m.MigrateToChatID, m.MigrateFromChatID)
		}
		if !m.IsTopicMessage {
			t.Errorf("%s: is_topic_message lost", name)
		}
		if m.ReplyToMessage == nil || m.ReplyToMessage.Sticker == nil || m.ReplyToMessage.Sticker.FileID != "CAAC" {
			t.Errorf("%s: nested reply = %+v", name, m.ReplyToMessage)
		}
		if m.ReplyToMessage != nil && len(m.ReplyToMessage.Raw) == 0 {
			t.Errorf("%s: a nested message keeps its own Raw", name)
		}
		// Raw is the message as it arrived, including what is not modeled.
		var arrived map[string]json.RawMessage
		if err := json.Unmarshal(m.Raw, &arrived); err != nil {
			t.Fatalf("%s: Raw is not JSON: %v", name, err)
		}
		for _, key := range []string{"animation", "sticker", "migrate_to_chat_id", "is_topic_message", "edit_date"} {
			if _, ok := arrived[key]; !ok {
				t.Errorf("%s: Raw lost %q", name, key)
			}
		}
		if string(m.Raw) != message {
			t.Errorf("%s: Raw differs from what arrived:\n got %s\nwant %s", name, m.Raw, message)
		}
	}

	var direct Message
	if err := json.Unmarshal([]byte(message), &direct); err != nil {
		t.Fatalf("decode Message: %v", err)
	}
	check(t, "message", &direct)

	for _, kind := range []string{"message", "edited_message"} {
		var u Update
		if err := json.Unmarshal([]byte(`{"update_id":1,"`+kind+`":`+message+`}`), &u); err != nil {
			t.Fatalf("decode Update %s: %v", kind, err)
		}
		if kind == "message" {
			check(t, "update.message", u.Message)
		} else {
			check(t, "update.edited_message", u.EditedMessage)
		}
	}
}

// GetUpdates is where these used to vanish: an update of a kind the struct did
// not know decoded into an Update with nothing but its id, and no error.
func TestGetUpdatesKeepsEditsAndReactions(t *testing.T) {
	var allowed string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		allowed = r.URL.Query().Get("allowed_updates")
		_, _ = w.Write([]byte(`{"ok":true,"result":[` +
			`{"update_id":10,"edited_message":{"message_id":1,"chat":` + sampleSupergroup + `,"date":1760000000,"edit_date":1760000010,"text":"v2"}},` +
			`{"update_id":11,"message_reaction":{"chat":` + sampleSupergroup + `,"message_id":1,"user":` + sampleUser + `,"date":1760000020,` +
			`"old_reaction":[],"new_reaction":[{"type":"emoji","emoji":"❤"}]}},` +
			`{"update_id":12,"message_reaction_count":{"chat":` + sampleSupergroup + `,"message_id":1,"date":1760000030,` +
			`"reactions":[{"type":{"type":"emoji","emoji":"❤"},"total_count":2}]}}]}`))
	}, WithAllowedUpdates("message", "edited_message", "callback_query", "my_chat_member", "message_reaction", "message_reaction_count"))

	updates, err := c.GetUpdates(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("GetUpdates: %v", err)
	}
	if len(updates) != 3 {
		t.Fatalf("updates = %d, want 3", len(updates))
	}
	if updates[0].EditedMessage == nil || updates[0].EditedMessage.Text != "v2" {
		t.Errorf("update 10 = %+v, want an edited message", updates[0])
	}
	if updates[1].MessageReaction == nil || len(updates[1].MessageReaction.NewReaction) != 1 {
		t.Errorf("update 11 = %+v, want a reaction", updates[1])
	}
	if updates[2].MessageReactionCount == nil || updates[2].MessageReactionCount.Reactions[0].TotalCount != 2 {
		t.Errorf("update 12 = %+v, want a reaction count", updates[2])
	}
	const wantAllowed = `["message","edited_message","callback_query","my_chat_member","message_reaction","message_reaction_count"]`
	if allowed != wantAllowed {
		t.Errorf("allowed_updates = %s, want %s", allowed, wantAllowed)
	}
}

// Telegram's required fields stay on the wire at their zero value and the
// optional ones stay off it, like every other type here.
func TestNewTypesMarshalLikeTheirBotAPIShape(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"paid reaction", ReactionType{Type: ReactionPaid}, `{"type":"paid"}`},
		{"emoji reaction", ReactionType{Type: ReactionEmoji, Emoji: "👍"}, `{"type":"emoji","emoji":"👍"}`},
		{"custom emoji reaction", ReactionType{Type: ReactionCustomEmoji, CustomEmojiID: "5368"}, `{"type":"custom_emoji","custom_emoji_id":"5368"}`},
		{
			"count of zero", ReactionCount{Type: ReactionType{Type: ReactionEmoji, Emoji: "👍"}},
			`{"type":{"type":"emoji","emoji":"👍"},"total_count":0}`,
		},
		{
			"animation", Animation{FileID: "f", FileUniqueID: "u"},
			`{"file_id":"f","file_unique_id":"u","width":0,"height":0,"duration":0}`,
		},
		{
			"sticker", Sticker{FileID: "f", FileUniqueID: "u", Type: "regular"},
			`{"file_id":"f","file_unique_id":"u","type":"regular","width":0,"height":0,"is_animated":false,"is_video":false}`,
		},
		{
			"reaction update without a user", MessageReactionUpdated{
				Chat: Chat{ID: -1, Type: ChatSupergroup}, MessageID: 2, Date: 3,
				OldReaction: []ReactionType{}, NewReaction: []ReactionType{{Type: ReactionPaid}},
			},
			`{"chat":{"id":-1,"type":"supergroup"},"message_id":2,"date":3,"old_reaction":[],"new_reaction":[{"type":"paid"}]}`,
		},
		{
			"chat member without is_member", ChatMember{Status: "member", User: User{ID: 1, FirstName: "A"}},
			`{"status":"member","user":{"id":1,"is_bot":false,"first_name":"A"}}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			if string(raw) != tt.want {
				t.Fatalf("marshaled\n got %s\nwant %s", raw, tt.want)
			}
		})
	}
}
