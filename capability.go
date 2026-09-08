// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// A Bot API server does not report its version anywhere a bot can read it:
// getMe says nothing about it, the statistics port carries uptime and memory
// but no version, and a containerized server logs nothing at all. What it does
// report, unambiguously, is whether a method exists:
//
//	POST sendMessage      {}  ->  400 Bad Request: message text is empty
//	POST sendRichMessage  {}  ->  404 Not Found: method not found
//
// So the way to find out whether a server can serve this bot is to ask it for
// the methods the bot cannot work without, before the bot starts working.

// probeSafe lists methods that cannot do anything with an empty body: they all
// fail parameter validation first. Probing is a real call, so a method that
// could succeed on its own must never appear here — getMe would simply run,
// deleteWebhook would drop a webhook, and logOut would detach the bot from the
// server and start a ten-minute cooldown.
var probeSafe = map[string]bool{
	"answerCallbackQuery":      true,
	"answerInlineQuery":        true,
	"copyMessage":              true,
	"deleteEphemeralMessage":   true,
	"deleteMessage":            true,
	"editEphemeralMessageText": true,
	"editMessageCaption":       true,
	"editMessageReplyMarkup":   true,
	"editMessageText":          true,
	"forwardMessage":           true,
	"getChat":                  true,
	"getChatMember":            true,
	"getFile":                  true,
	"sendAudio":                true,
	"sendChatAction":           true,
	"sendDocument":             true,
	"sendMessage":              true,
	"sendMessageDraft":         true,
	"sendPhoto":                true,
	"sendRichMessage":          true,
	"sendRichMessageDraft":     true,
	"sendVideo":                true,
	"sendVoice":                true,
	"setMessageReaction":       true,
}

// MissingMethodsError reports methods the server does not implement. It is
// what a bot should refuse to start on.
type MissingMethodsError struct {
	// Methods are the missing method names, sorted.
	Methods []string
	// APIBase is the server that was asked.
	APIBase string
}

func (e *MissingMethodsError) Error() string {
	return fmt.Sprintf("telegram server %s does not implement: %s (this bot is written against Bot API %s)",
		e.APIBase, strings.Join(e.Methods, ", "), BotAPI)
}

// Probe reports which of the given methods the server does not implement. A
// method is asked for with an empty body: an existing method rejects that on
// its parameters, a missing one answers "method not found".
//
// Every method must be in the safe list, otherwise Probe refuses without
// making any call: probing is not a dry run, and a method that works without
// parameters would actually execute.
func (c *Client) Probe(ctx context.Context, methods ...string) ([]string, error) {
	for _, m := range methods {
		if !probeSafe[m] {
			return nil, fmt.Errorf("refusing to probe %q: probing calls the method for real, and only methods that fail without parameters are safe to ask for", m)
		}
	}
	var missing []string
	for _, m := range methods {
		var discard map[string]any
		err := c.post(ctx, m, map[string]any{}, &discard)
		switch {
		case err == nil:
			// The method exists and, surprisingly, accepted an empty body.
			continue
		case IsMethodNotFound(err):
			missing = append(missing, m)
		case errors.As(err, new(*APIError)):
			// Any other API answer (400, 401, 403…) means the method is there.
			continue
		default:
			return nil, err
		}
	}
	sort.Strings(missing)
	return missing, nil
}

// Require is Probe as a single error.
func (c *Client) Require(ctx context.Context, methods ...string) error {
	missing, err := c.Probe(ctx, methods...)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return &MissingMethodsError{Methods: missing, APIBase: c.apiBase}
	}
	return nil
}

// Needs describes what a bot cannot run without.
type Needs struct {
	// Methods are Bot API methods whose absence makes the bot useless. They
	// must be probe-safe; see [Client.Probe].
	Methods []string
	// Files requires a readable and writable data directory for this bot on a
	// --local server; see [WithLocalFiles].
	Files bool
	// Wait allows the server to still be starting up: getMe is retried until
	// this much time has passed. A self-hosted server shares a lifecycle with
	// the bot and often loses the race by a few seconds.
	Wait time.Duration
}

// Preflight is the one call a bot makes at startup. It answers the question
// that today's failure modes hide: can the server on the other end actually
// serve this bot? It returns the bot's own identity, which the caller needs
// anyway for @mentions and command scoping.
//
// A bot should treat any error from Preflight as fatal. Failing to start with
// a named cause is the point: the alternative is a bot that polls happily and
// answers nothing.
func (c *Client) Preflight(ctx context.Context, n Needs) (Me, error) {
	me, err := c.getMeWaiting(ctx, n.Wait)
	if err != nil {
		return Me{}, err
	}
	if len(n.Methods) > 0 {
		if err := c.Require(ctx, n.Methods...); err != nil {
			return Me{}, err
		}
	}
	if n.Files {
		if err := c.VerifyLocalFiles(); err != nil {
			return Me{}, err
		}
	}
	return me, nil
}

// getMeWaiting retries getMe until wait has passed. Only connection-level
// failures are worth waiting on: a rejected token will not become valid.
func (c *Client) getMeWaiting(ctx context.Context, wait time.Duration) (Me, error) {
	deadline := time.Now().Add(wait)
	delay := 500 * time.Millisecond
	for attempt := 1; ; attempt++ {
		me, err := c.GetMe(ctx)
		if err == nil {
			return me, nil
		}
		var apiErr *APIError
		if errors.As(err, &apiErr) || time.Now().After(deadline) || ctx.Err() != nil {
			return Me{}, err
		}
		c.log.Warn("telegram bot api not ready, retrying getMe",
			"attempt", attempt, "retry_in", delay.String(), "error", err)
		if err := c.sleep(ctx, delay); err != nil {
			return Me{}, err
		}
		if delay < 5*time.Second {
			delay *= 2
		}
	}
}
