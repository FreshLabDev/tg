package tg

import (
	"encoding/json"
	"testing"
)

// A message re-encoded for an audit record should look like what Telegram
// sends, not like a struct with every field this package models.
func TestMarshalOmitsAbsentFields(t *testing.T) {
	raw, err := json.Marshal(Message{
		MessageID: 3, Date: 1,
		Chat: Chat{ID: 7, Type: "private"},
		From: &User{ID: 7, IsBot: false, FirstName: "A"},
		Text: "hi",
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"message_id":3,"from":{"id":7,"is_bot":false,"first_name":"A"},"chat":{"id":7,"type":"private"},"date":1,"text":"hi"}`
	if string(raw) != want {
		t.Fatalf("marshaled\n got %s\nwant %s", raw, want)
	}
}

// A field Telegram always sends must always marshal, even at its zero value:
// a button whose label came out empty has to be sent as an empty label, not as
// a button with no label field at all.
func TestRequiredFieldsSurviveTheirZeroValue(t *testing.T) {
	raw, err := json.Marshal(InlineKeyboardButton{Text: "", CallbackData: "noop"})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"text":"","callback_data":"noop"}` {
		t.Fatalf("button = %s", raw)
	}

	raw, _ = json.Marshal(MessageEntity{Type: "spoiler", Offset: 6, Length: 0})
	if string(raw) != `{"type":"spoiler","offset":6,"length":0}` {
		t.Fatalf("entity = %s", raw)
	}

	raw, _ = json.Marshal(Voice{FileID: "f", FileUniqueID: "u", Duration: 0})
	if string(raw) != `{"file_id":"f","file_unique_id":"u","duration":0}` {
		t.Fatalf("voice = %s", raw)
	}
}

// An update carries exactly one of its members; the others are not nulls this
// package invented.
func TestUpdateMarshalsOnlyWhatItCarries(t *testing.T) {
	raw, err := json.Marshal(Update{UpdateID: 5})
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"update_id":5}` {
		t.Fatalf("update = %s", raw)
	}
}
