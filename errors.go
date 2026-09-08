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
	// Response is the error body as Telegram sent it, for a bot that records
	// what it was told rather than this package's reading of it. It carries no
	// token: Telegram does not echo the request.
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

// IsUnreachableDestination reports a permanent Telegram error meaning the chat
// can never receive messages again (blocked, kicked, deleted, deactivated).
// Matched on the human description because Telegram overloads 400/403 across
// many cases.
func (e *APIError) IsUnreachableDestination() bool {
	if e == nil || (e.StatusCode != http.StatusBadRequest && e.StatusCode != http.StatusForbidden) {
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
	return apiErr.StatusCode == http.StatusNotFound &&
		strings.Contains(strings.ToLower(apiErr.Description), "method not found")
}

// IsTooManyRequests reports a 429. RetryAfter carries how long Telegram asked
// the caller to wait.
func IsTooManyRequests(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests
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

type transportError struct {
	msg   string
	cause error
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
	cause := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		cause = urlErr.Err
	}
	msg := cause.Error()
	if c.token != "" {
		msg = strings.ReplaceAll(msg, c.token, "***")
	}
	return &transportError{msg: "telegram transport failed: " + msg, cause: cause}
}

func parseAPIError(method string, statusCode int, body io.Reader) *APIError {
	raw, _ := io.ReadAll(io.LimitReader(body, 4096))
	var payload struct {
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
		Parameters  struct {
			RetryAfter      int   `json:"retry_after"`
			MigrateToChatID int64 `json:"migrate_to_chat_id"`
		} `json:"parameters"`
	}
	description := strings.TrimSpace(string(raw))
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
