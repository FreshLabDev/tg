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
