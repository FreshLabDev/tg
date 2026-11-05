// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"encoding/json"
	"testing"
)

func TestUTF16Len(t *testing.T) {
	for s, want := range map[string]int{
		"":        0,
		"abc":     3,
		"привет":  6, // two bytes each in UTF-8, one unit each in UTF-16
		"🔥":       2, // outside the BMP: a surrogate pair
		"👍🏽":      4, // a thumb and a skin tone, two pairs
		"🔥 x.com": 8,
		"\xff":    1, // invalid UTF-8 reads as U+FFFD
		"日本語テキスト": 7,
	} {
		if got := UTF16Len(s); got != want {
			t.Errorf("UTF16Len(%q) = %d, want %d", s, got, want)
		}
	}
}

// The link after an emoji: Telegram's offset counts the emoji twice, so a
// slice by runes starts one character late and by bytes two characters
// early. The entity is exactly what Telegram sends for this text.
func TestEntityTextAfterAnEmoji(t *testing.T) {
	var msg Message
	raw := `{"message_id":1,"date":1,"chat":{"id":1,"type":"private"},
		"text":"🔥 x.com/a/status/1 и 🇺🇦 https://t.me/x",
		"entities":[{"type":"url","offset":3,"length":16},{"type":"url","offset":27,"length":14}]}`
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	if got := EntityText(msg.Text, msg.Entities[0]); got != "x.com/a/status/1" {
		t.Fatalf("first entity = %q", got)
	}
	if got := EntityText(msg.Text, msg.Entities[1]); got != "https://t.me/x" {
		t.Fatalf("second entity = %q, after a flag (two pairs) and Cyrillic", got)
	}
}

func TestEntityTextThatDoesNotFit(t *testing.T) {
	text := "🔥 link"
	for name, e := range map[string]MessageEntity{
		"inside the emoji":       {Offset: 1, Length: 3},
		"ends inside the emoji":  {Offset: 0, Length: 1},
		"past the end":           {Offset: 3, Length: 5},
		"starts past the end":    {Offset: 8, Length: 1},
		"negative offset":        {Offset: -1, Length: 2},
		"negative length":        {Offset: 0, Length: -1},
		"empty at the very end":  {Offset: 7, Length: 0},
		"empty inside the emoji": {Offset: 1, Length: 0},
	} {
		if got := EntityText(text, e); got != "" {
			t.Errorf("%s: EntityText = %q, want empty", name, got)
		}
	}
	if got := EntityText(text, MessageEntity{Offset: 0, Length: 7}); got != text {
		t.Fatalf("the whole text = %q", got)
	}
	if got := EntityText(text, MessageEntity{Offset: 0, Length: 2}); got != "🔥" {
		t.Fatalf("the emoji = %q", got)
	}
}
