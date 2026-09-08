// SPDX-License-Identifier: Apache-2.0
package tg

import "strings"

// MaxMessageRunes is Telegram's limit for sendMessage text. Rich messages are
// not bound by it, which is why long output belongs in one of those.
const MaxMessageRunes = 4096

// SplitText cuts plain text into parts of at most limit runes, preferring a
// line break and then a space near the end of each part, so words and lines
// survive. Runes are never split.
//
// It deliberately knows nothing about HTML: cutting markup safely means
// closing the open tags at the cut and reopening them after it, which needs a
// parser and a decision about which tags a bot allows. A caller that escapes
// or wraps its text must measure the result it actually sends — see how voicy
// splits an escaped, quoted transcript — rather than assume this function's
// count matches.
func SplitText(text string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	var out []string
	for len(runes) > 0 {
		if len(runes) <= limit {
			if part := strings.TrimSpace(string(runes)); part != "" {
				out = append(out, part)
			}
			break
		}
		cut := limit
		// Look back over the last 40% of the budget for a natural boundary: a
		// line break first, a space second. Anything earlier would waste too
		// much of the message.
		floor := limit * 3 / 5
		if i := lastIndexIn(runes, floor, limit, '\n'); i >= 0 {
			cut = i + 1
		} else if i := lastIndexIn(runes, floor, limit, ' '); i >= 0 {
			cut = i + 1
		}
		if part := strings.TrimSpace(string(runes[:cut])); part != "" {
			out = append(out, part)
		}
		runes = runes[cut:]
	}
	return out
}

func lastIndexIn(runes []rune, from, to int, target rune) int {
	for i := to - 1; i >= from; i-- {
		if runes[i] == target {
			return i
		}
	}
	return -1
}
