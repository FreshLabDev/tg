// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// APIError is a non-2xx answer from the Bot API.
type APIError struct {
	Method      string
	StatusCode  int
	ErrorCode   int
	Description string
	RetryAfter  time.Duration
	// MigrateToChatID is set when Telegram reports that a group was upgraded to
	// a supergroup (parameters.migrate_to_chat_id): the old chat_id is dead and
	// this is the chat_id to use instead.
	MigrateToChatID int64
	// Response is the error body as it arrived, for a bot that records what it
	// was told rather than this package's reading of it. Telegram never echoes
	// the request, but a proxy answering 404 or 502 in its place quotes the
	// request URI -- which is the token -- so this is redacted like a URL.
	Response json.RawMessage
}

func (e *APIError) Error() string {
	description := strings.TrimSpace(e.Description)
	if description == "" {
		description = "request failed"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("telegram %s failed: %s (retry after %s)", e.Method, description, e.RetryAfter)
	}
	return fmt.Sprintf("telegram %s failed: %s", e.Method, description)
}

// HTTPStatus returns the HTTP status so callers can classify a failure without
// importing this package's concrete type.
func (e *APIError) HTTPStatus() int { return e.StatusCode }

// code is what the failure means. Telegram normally answers with the same
// number twice, as an HTTP status and as error_code; when a proxy returns 200
// around an ok=false body, only error_code carries the meaning.
func (e *APIError) code() int {
	if e.ErrorCode != 0 {
		return e.ErrorCode
	}
	return e.StatusCode
}

// IsUnreachableDestination reports a permanent Telegram error meaning the chat
// can never receive messages again (blocked, kicked, deleted, deactivated).
// Matched on the human description because Telegram overloads 400/403 across
// many cases.
func (e *APIError) IsUnreachableDestination() bool {
	if e == nil || (e.code() != http.StatusBadRequest && e.code() != http.StatusForbidden) {
		return false
	}
	if e.MigrateToChatID != 0 {
		return true
	}
	desc := strings.ToLower(e.Description)
	for _, marker := range []string{
		"bot was blocked",
		"user is deactivated",
		"bot was kicked",
		"bot is not a member",
		"chat not found",
		"group chat was deleted",
		"group chat was upgraded to a supergroup",
		"need administrator rights",
	} {
		if strings.Contains(desc, marker) {
			return true
		}
	}
	return false
}

// IsMessageNotModified reports Telegram's benign "message is not modified",
// which happens whenever an inline-keyboard tap re-renders identical text. It
// should be treated as success so the bot does not post a duplicate.
func IsMessageNotModified(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return strings.Contains(strings.ToLower(apiErr.Description), "message is not modified")
	}
	return false
}

// IsMethodNotFound reports that the server does not implement the method at
// all — the answer a Bot API server older than this module gives to everything
// it has not learned yet. It is what [Client.Probe] is built on, and the reason
// a bot can silently stop replying instead of failing.
func IsMethodNotFound(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.code() == http.StatusNotFound &&
		strings.Contains(strings.ToLower(apiErr.Description), "method not found")
}

// IsTooManyRequests reports a 429. RetryAfter carries how long Telegram asked
// the caller to wait.
func IsTooManyRequests(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.code() == http.StatusTooManyRequests
}

// RetryAfter returns the delay Telegram asked for, or zero.
func RetryAfter(err error) time.Duration {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.RetryAfter
	}
	return 0
}

// ErrFileTooLarge marks a limit that a second attempt cannot get past.
var ErrFileTooLarge = errors.New("telegram file exceeds the download limit")

// redactAPI scrubs the token out of an error body. Telegram does not echo the
// request, but a reverse proxy or a sidecar in front of a self-hosted server
// answers with the request URI in the body -- and that body reaches log lines
// and, in one bot, a database column.
func (c *Client) redactAPI(e *APIError) *APIError {
	if e == nil || c.token == "" {
		return e
	}
	e.Description = c.redact(e.Description)
	if len(e.Response) > 0 {
		e.Response = json.RawMessage(c.redact(string(e.Response)))
	}
	return e
}

// ErrUnexpectedResult means Telegram accepted the request -- it answered 2xx
// with ok=true -- but the result was not what the method returns. The action
// happened; only the answer is unreadable.
//
// The distinction matters to a caller that must not repeat itself. A bot whose
// notification was delivered should not queue it again because the receipt was
// unparseable, while a bot that needs the new message's id to edit it later
// genuinely cannot continue. So this is an error, and a caller that can live
// without the result checks for it:
//
//	if err != nil && !errors.Is(err, tg.ErrUnexpectedResult) {
//		return err
//	}
var ErrUnexpectedResult = errors.New("telegram returned an unexpected result")

type transportError struct {
	msg   string
	cause error
	// unsent is true when the request certainly did not reach Telegram: it
	// was never written whole. False means it may have.
	unsent bool
}

func (e *transportError) Error() string { return e.msg }
func (e *transportError) Unwrap() error { return e.cause }

// redactError rewrites a request or transport error so its message can never
// contain the bot token: net/http and net/url errors embed the full request
// URL, which includes the /bot<TOKEN>/ path segment. The underlying cause stays
// wrapped so errors.Is and errors.As keep working, but the URL-bearing wrapper
// itself is dropped from the chain.
func (c *Client) redactError(err error) error {
	if err == nil {
		return nil
	}
	return c.transportFailure(err, false)
}

// transportFailure is redactError for a failed attempt at a Bot API method,
// which also records whether the request certainly never left.
func (c *Client) transportFailure(err error, unsent bool) *transportError {
	cause := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		cause = urlErr.Err
	}
	msg := cause.Error()
	if c.token != "" {
		msg = strings.ReplaceAll(msg, c.token, "***")
	}
	return &transportError{msg: "telegram transport failed: " + msg, cause: cause, unsent: unsent}
}

// OutcomeUnknown reports whether a failed call may still have taken effect, so
// that making it again could do the thing twice: Telegram answered 5xx, its
// answer could not be read ([ErrUnexpectedResult], where the call did take
// effect), or the connection failed after the whole request was written.
//
// It is false for a nil error and for every failure known to have done
// nothing: a refusal (4xx, 429 included), a request that was never written
// whole (the host did not resolve, the connection was refused, the upload
// broke off or timed out halfway), and a call refused before any request, such
// as an album with no items.
//
// A sender that must not post a message twice asks this before it repeats a
// send: true means the message may already be in the chat. A request the
// transport could not follow -- a custom RoundTripper that reports no
// httptrace events -- counts as possibly delivered unless the failure was in
// resolving or dialing.
func OutcomeUnknown(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.code() >= 500
	}
	if errors.Is(err, ErrUnexpectedResult) {
		return true
	}
	var transport *transportError
	if errors.As(err, &transport) {
		return !transport.unsent
	}
	return false
}

// badRequest reports a 400 whose description holds one of markers. Telegram
// answers 400 for most unrelated mistakes, so the description is the only
// thing that tells them apart; markers are lowercase.
func badRequest(err error, markers ...string) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.code() != http.StatusBadRequest {
		return false
	}
	desc := strings.ToLower(apiErr.Description)
	for _, marker := range markers {
		if strings.Contains(desc, marker) {
			return true
		}
	}
	return false
}

// IsMessageGone reports that the message an edit or a delete addressed is out
// of the bot's reach: deleted ("message to edit not found", "message to
// delete not found"), or one Telegram no longer lets it change ("message can't
// be edited", "message can't be deleted", for everyone included). Repeating
// the call fails the same way, so a bot that tidies up after itself counts
// the message as done.
func IsMessageGone(err error) bool {
	return badRequest(err,
		"message to edit not found",
		"message to delete not found",
		"message can't be edited",
		"message can't be deleted",
	)
}

// IsInvalidFileID reports that Telegram could not use the file a request named
// by a string, an [InputFileString]: a file_id it does not know ("wrong file
// identifier", "wrong remote file identifier"), or a URL it could not fetch or
// could not use ("failed to get HTTP URL content", "wrong HTTP URL", "wrong
// type of the web page content"). The same string fails the same way next
// time, so a bot that caches file_ids drops it and sends the file itself.
//
// A file that is too large is not this: the identifier is fine, the file is
// not, and whether to forget it is the bot's call.
func IsInvalidFileID(err error) bool {
	return badRequest(err,
		"wrong file identifier",
		"wrong remote file identifier",
		"failed to get http url content",
		"wrong http url",
		"invalid http url",
		"wrong type of the web page content",
	)
}

// IsCantParseEntities reports that Telegram refused the HTML of a text or a
// caption: an unclosed or unknown tag, or an unescaped "<" or "&". Sending the
// same markup again fails the same way.
func IsCantParseEntities(err error) bool {
	return badRequest(err, "can't parse entities")
}

// IsCaptionTooLong reports that a caption was over Telegram's limit. The limit
// applies to the text after the HTML is parsed, so tags do not count.
func IsCaptionTooLong(err error) bool {
	return badRequest(err, "caption is too long", "media_caption_too_long")
}

// IsLocalFileUnreadable reports that a server started with --local could not
// use the file at a path it was given ([InputFileLocal]): there is nothing at
// the path as the server sees it, it is not a regular file, or it is empty.
// The usual cause is a volume mounted at a different path in the server's
// container than in the bot's. An empty upload is refused with the same words.
func IsLocalFileUnreadable(err error) bool {
	return badRequest(err,
		"can't find real file path",
		"can't get stat about the file",
		"file must be a regular file",
		"file must have non-empty path",
		"file must be non-empty",
	)
}

// refusal returns an APIError when a 2xx body says ok=false, and nil when the
// answer is what it claims to be.
func refusal(method string, statusCode int, raw []byte) *APIError {
	var envelope struct {
		OK *bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil // let the caller's own decode produce the error
	}
	if envelope.OK == nil || *envelope.OK {
		return nil
	}
	return apiErrorFrom(method, statusCode, raw)
}

func parseAPIError(method string, statusCode int, body io.Reader) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	return apiErrorFrom(method, statusCode, raw)
}

func apiErrorFrom(method string, statusCode int, raw []byte) *APIError {
	var payload struct {
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter      int   `json:"retry_after"`
			MigrateToChatID int64 `json:"migrate_to_chat_id"`
		} `json:"parameters"`
	}
	description := strings.TrimSpace(string(raw))
	if len(description) > 4096 {
		description = description[:4096]
	}
	if err := json.Unmarshal(raw, &payload); err == nil && payload.Description != "" {
		description = payload.Description
	}
	apiErr := &APIError{
		Method:          method,
		StatusCode:      statusCode,
		ErrorCode:       payload.ErrorCode,
		Description:     description,
		MigrateToChatID: payload.Parameters.MigrateToChatID,
	}
	if len(raw) > 0 {
		apiErr.Response = append(json.RawMessage(nil), raw...)
	}
	if payload.Parameters.RetryAfter > 0 {
		apiErr.RetryAfter = time.Duration(payload.Parameters.RetryAfter) * time.Second
	}
	return apiErr
}
