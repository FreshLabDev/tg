// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"strings"
	"testing"
)

func TestSplitTextKeepsShortTextWhole(t *testing.T) {
	got := SplitText("hello", MaxMessageRunes)
	if len(got) != 1 || got[0] != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitTextPrefersLineBreaks(t *testing.T) {
	text := strings.Repeat("a", 30) + "\n" + strings.Repeat("b", 30)
	got := SplitText(text, 40)
	if len(got) != 2 {
		t.Fatalf("parts = %d (%q), want 2", len(got), got)
	}
	if got[0] != strings.Repeat("a", 30) || got[1] != strings.Repeat("b", 30) {
		t.Fatalf("cut in the wrong place: %q", got)
	}
}

func TestSplitTextFallsBackToSpaces(t *testing.T) {
	text := strings.Repeat("word ", 20)
	for _, part := range SplitText(text, 30) {
		if len([]rune(part)) > 30 {
			t.Fatalf("part longer than the limit: %q", part)
		}
		if strings.HasPrefix(part, " ") || strings.HasSuffix(part, " ") {
			t.Fatalf("part not trimmed: %q", part)
		}
	}
}

// A hard cut must still land on a rune boundary, never inside a character.
func TestSplitTextNeverBreaksARune(t *testing.T) {
	text := strings.Repeat("кружочек", 40)
	parts := SplitText(text, 25)
	if len(parts) < 2 {
		t.Fatal("want several parts")
	}
	if strings.Join(parts, "") != text {
		t.Fatal("text without spaces must reassemble exactly")
	}
	for _, part := range parts {
		if len([]rune(part)) > 25 {
			t.Fatalf("part of %d runes exceeds the limit", len([]rune(part)))
		}
	}
}

func TestSplitTextEdgeCases(t *testing.T) {
	if got := SplitText("", 10); got != nil {
		t.Fatalf("empty text = %q, want nil", got)
	}
	if got := SplitText("x", 0); got != nil {
		t.Fatalf("zero limit = %q, want nil", got)
	}
}
