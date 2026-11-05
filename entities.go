// SPDX-License-Identifier: Apache-2.0
package tg

// Telegram measures a message's text in UTF-16 code units: the offset and the
// length of every [MessageEntity] count them. A Go string is UTF-8 bytes, and a
// rune is one code point, while a character outside the Basic Multilingual
// Plane -- most emoji -- is two code units. Slicing the text by an entity's
// numbers as bytes or as runes is right until the first such character and
// wrong after it, which is how a link after an emoji gets lost.

// UTF16Len is the length of s in UTF-16 code units, the unit of every entity
// offset and length.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Width(r)
	}
	return n
}

// EntityText returns the part of text an entity covers, the text being the
// message's Text for its Entities or its Caption for its CaptionEntities. An
// entity that does not fit text, or whose bounds fall inside a character,
// yields "".
func EntityText(text string, e MessageEntity) string {
	if e.Offset < 0 || e.Length < 0 {
		return ""
	}
	start := utf16Index(text, e.Offset)
	if start < 0 {
		return ""
	}
	end := utf16Index(text[start:], e.Length)
	if end < 0 {
		return ""
	}
	return text[start : start+end]
}

// utf16Index returns the byte offset in s after units UTF-16 code units, or -1
// when that falls inside a character or past the end.
func utf16Index(s string, units int) int {
	n := 0
	for i, r := range s {
		if n == units {
			return i
		}
		if n > units {
			return -1
		}
		n += utf16Width(r)
	}
	if n == units {
		return len(s)
	}
	return -1
}

// utf16Width is how many UTF-16 code units r takes. Invalid UTF-8 decodes as
// U+FFFD, which takes one, as it does once Telegram has replaced it.
func utf16Width(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}
