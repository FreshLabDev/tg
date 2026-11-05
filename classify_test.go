// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The descriptions are what the Bot API server answers, as its source and
// TDLib's word them; the server lowercases the first letter after the prefix.
func TestErrorClassifiers(t *testing.T) {
	refusal := func(status int, description string) error {
		body := fmt.Sprintf(`{"ok":false,"error_code":%d,"description":%q}`, status, description)
		return parseAPIError("method", status, strings.NewReader(body))
	}
	type classifier struct {
		name string
		is   func(error) bool
	}
	gone := classifier{"IsMessageGone", IsMessageGone}
	invalid := classifier{"IsInvalidFileID", IsInvalidFileID}
	entities := classifier{"IsCantParseEntities", IsCantParseEntities}
	caption := classifier{"IsCaptionTooLong", IsCaptionTooLong}
	local := classifier{"IsLocalFileUnreadable", IsLocalFileUnreadable}

	for _, tc := range []struct {
		err  error
		is   classifier
		want bool
	}{
		{refusal(400, "Bad Request: message to edit not found"), gone, true},
		{refusal(400, "Bad Request: message to delete not found"), gone, true},
		{refusal(400, "Bad Request: message can't be edited"), gone, true},
		{refusal(400, "Bad Request: message can't be deleted for everyone"), gone, true},
		{refusal(400, "Bad Request: message is not modified: specified new message content and reply markup are exactly the same"), gone, false},
		{refusal(400, "Bad Request: message media can't be edited"), gone, false},
		{refusal(400, "Bad Request: message to be replied not found"), gone, false},
		{refusal(403, "Forbidden: bot was blocked by the user"), gone, false},

		{refusal(400, "Bad Request: wrong file identifier/HTTP URL specified"), invalid, true},
		{refusal(400, "Bad Request: wrong remote file identifier specified: Wrong padding length"), invalid, true},
		{refusal(400, "Bad Request: failed to get HTTP URL content"), invalid, true},
		{refusal(400, "Bad Request: wrong HTTP URL specified"), invalid, true},
		{refusal(400, "Bad Request: wrong type of the web page content"), invalid, true},
		{refusal(400, "Bad Request: file is too big"), invalid, false},
		{refusal(400, "Bad Request: chat not found"), invalid, false},

		{refusal(400, "Bad Request: can't parse entities: Can't find end of the entity starting at byte offset 12"), entities, true},
		{refusal(400, "Bad Request: message text is empty"), entities, false},

		{refusal(400, "Bad Request: message caption is too long"), caption, true},
		{refusal(400, "Bad Request: MEDIA_CAPTION_TOO_LONG"), caption, true},
		{refusal(400, "Bad Request: message is too long"), caption, false},

		{refusal(400, "Bad Request: can't find real file path"), local, true},
		{refusal(400, "Bad Request: file must be a regular file"), local, true},
		{refusal(400, "Bad Request: file must be non-empty"), local, true},
		{refusal(400, "Bad Request: wrong file identifier/HTTP URL specified"), local, false},

		// A wrapped refusal is still that refusal; anything else is none.
		{fmt.Errorf("deliver: %w", refusal(400, "Bad Request: message to delete not found")), gone, true},
		{errors.New("message to delete not found"), gone, false},
		{nil, invalid, false},
	} {
		if got := tc.is.is(tc.err); got != tc.want {
			t.Errorf("%s(%v) = %v, want %v", tc.is.name, tc.err, got, tc.want)
		}
	}
}

// Only a 400 counts: the same words from a proxy's 502 page are not a
// refusal of the request.
func TestClassifiersWantABadRequest(t *testing.T) {
	err := parseAPIError("editMessageText", http.StatusBadGateway, strings.NewReader("message to edit not found"))
	if IsMessageGone(err) {
		t.Fatal("a 502 read as a deleted message")
	}
}
