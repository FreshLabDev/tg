// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// oldServer answers the way a Bot API 7.11 server does: methods it knows fail
// on their parameters, methods from a later version do not exist.
func oldServer(known map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if known[method] {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message text is empty"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":404,"description":"Not Found: method not found"}`))
	}
}

func TestProbeSeparatesMissingFromPresent(t *testing.T) {
	c, _ := newTestClient(t, oldServer(map[string]bool{"sendMessage": true, "editMessageText": true}))

	missing, err := c.Probe(context.Background(), "sendMessage", "sendRichMessage", "editEphemeralMessageText")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	want := []string{"editEphemeralMessageText", "sendRichMessage"}
	if len(missing) != len(want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	for i := range want {
		if missing[i] != want[i] {
			t.Fatalf("missing = %v, want %v (sorted)", missing, want)
		}
	}
}

// Probing is a real call. A method that works without parameters would run,
// and logOut would detach the bot from its server for ten minutes.
func TestProbeRefusesMethodsThatCouldExecute(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	// setMyDescription and setMyShortDescription clear the description on an empty
	// body, and setChatMenuButton resets the menu button to the default.
	for _, method := range []string{
		"logOut", "getMe", "deleteWebhook", "close",
		"setMyDescription", "setMyShortDescription", "setChatMenuButton",
	} {
		if _, err := c.Probe(context.Background(), method); err == nil {
			t.Fatalf("Probe(%q) must be refused", method)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("refused probes still made %d requests", calls.Load())
	}
}

func TestRequireNamesEveryMissingMethod(t *testing.T) {
	c, _ := newTestClient(t, oldServer(map[string]bool{"sendMessage": true}))

	err := c.Require(context.Background(), "sendMessage", "sendRichMessage")
	if err == nil {
		t.Fatal("want an error when the server lacks a required method")
	}
	var missing *MissingMethodsError
	if !errors.As(err, &missing) {
		t.Fatalf("error type = %T, want *MissingMethodsError", err)
	}
	if !strings.Contains(err.Error(), "sendRichMessage") || !strings.Contains(err.Error(), BotAPI) {
		t.Fatalf("message = %q, want the method and the Bot API version this module targets", err.Error())
	}
}

func TestPreflightFailsOnAServerThatCannotServeTheBot(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"voicyin_bot"}}`))
			return
		}
		oldServer(map[string]bool{"sendMessage": true})(w, r)
	})

	if _, err := c.Preflight(context.Background(), Needs{Methods: []string{"sendRichMessage"}}); err == nil {
		t.Fatal("Preflight must fail rather than let the bot poll and answer nothing")
	}

	me, err := c.Preflight(context.Background(), Needs{Methods: []string{"sendMessage"}})
	if err != nil {
		t.Fatalf("Preflight with a satisfied need: %v", err)
	}
	if me.Username != "voicyin_bot" {
		t.Fatalf("username = %q", me.Username)
	}
}

// A self-hosted server shares a lifecycle with the bot and often loses the
// race by a few seconds.
func TestPreflightWaitsForAServerThatIsStillStarting(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		// More failures than one GetMe absorbs internally (four attempts), so
		// the waiting loop itself has to run for this to pass.
		if calls.Add(1) <= 6 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"bot"}}`))
	})
	_ = srv
	c.sleep = func(context.Context, time.Duration) error { return nil }

	me, err := c.Preflight(context.Background(), Needs{Wait: time.Minute})
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	if me.Username != "bot" {
		t.Fatalf("username = %q", me.Username)
	}
}

// A bad token never becomes valid, so waiting on it is a waste of a startup.
func TestPreflightDoesNotWaitOnARejectedToken(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	if _, err := c.Preflight(context.Background(), Needs{Wait: time.Hour}); err == nil {
		t.Fatal("want the 401 surfaced immediately")
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}

// A self-hosted server that is still starting answers with a connection
// failure or a 5xx; a wrong token answers 401. Only the first is worth waiting
// on, and the earlier code waited on neither.
func TestPreflightWaitsThroughTransientAPIErrors(t *testing.T) {
	var calls atomic.Int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"ok":false,"error_code":503,"description":"Service Unavailable"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"bot"}}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	me, err := c.Preflight(context.Background(), Needs{Wait: time.Minute})
	if err != nil {
		t.Fatalf("Preflight through a 503: %v", err)
	}
	if me.Username != "bot" {
		t.Fatalf("username = %q", me.Username)
	}
}

// A probe carries an empty body, so the method cannot have run. One dropped
// connection must not read as "the server lacks this method".
func TestProbeRetriesADroppedConnection(t *testing.T) {
	var calls atomic.Int32
	c, srv := newTestClient(t, nil)
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			// Hang up without answering.
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server cannot hijack")
				return
			}
			conn, _, err := hijacker.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message text is empty"}`))
	})
	c.sleep = func(context.Context, time.Duration) error { return nil }

	missing, err := c.Probe(context.Background(), "sendMessage")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none: the method answered on the retry", missing)
	}
	if calls.Load() < 2 {
		t.Fatalf("calls = %d, want the probe replayed", calls.Load())
	}
}

// A rejected token is an answer about the caller, not about the method. The
// server never looked, so reporting "everything is present" would defeat the
// point of a guard whose job is to refuse to start.
func TestProbeRefusesToGuessWhenCredentialsAreRejected(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":401,"description":"Unauthorized"}`))
	})

	missing, err := c.Probe(context.Background(), "sendRichMessage")
	if err == nil {
		t.Fatalf("missing = %v, err = nil: a 401 must not read as 'the method is there'", missing)
	}
	if !strings.Contains(err.Error(), "cannot tell") {
		t.Fatalf("message = %q", err.Error())
	}
}

// A 409, a 429 or a 5xx is the server talking about itself: it was busy,
// conflicted with another poller or failing, and never looked at the request.
// Read as "the method is there" it lets a bot start against a server that lacks
// the method, and it makes a bot that falls back from rich messages latch onto
// them after one bad second at startup.
func TestProbeRefusesToGuessOnConflictRateLimitAndServerErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"conflict", http.StatusConflict, `{"ok":false,"error_code":409,"description":"Conflict: terminated by other getUpdates request"}`},
		{"rate limited", http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 1","parameters":{"retry_after":1}}`},
		{"rate limited without a retry_after", http.StatusTooManyRequests, `{"ok":false,"error_code":429,"description":"Too Many Requests"}`},
		{"internal error", http.StatusInternalServerError, `{"ok":false,"error_code":500,"description":"Internal Server Error"}`},
		{"bad gateway from a proxy", http.StatusBadGateway, `<html><body><h1>502 Bad Gateway</h1></body></html>`},
		{"service unavailable", http.StatusServiceUnavailable, `{"ok":false,"error_code":503,"description":"Service Unavailable"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			missing, err := c.Probe(context.Background(), "sendRichMessage")
			if err == nil {
				t.Fatalf("missing = %v, err = nil: a %d must not read as 'the method is there'", missing, tt.status)
			}
			if missing != nil {
				t.Fatalf("missing = %v, want no verdict alongside the error", missing)
			}
			if !strings.Contains(err.Error(), "cannot tell whether sendRichMessage exists") {
				t.Fatalf("message = %q", err.Error())
			}
			// The cause stays reachable, so a caller can still branch on it.
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
				t.Fatalf("error = %v, want the %d APIError wrapped", err, tt.status)
			}
		})
	}
}

// The answers that are about the method still are: a parameter error means the
// method exists, and "method not found" means it does not.
func TestProbeStillReadsParameterErrorsAndMissingMethods(t *testing.T) {
	c, _ := newTestClient(t, oldServer(map[string]bool{"sendMessage": true}))

	missing, err := c.Probe(context.Background(), "sendMessage", "sendRichMessage")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(missing) != 1 || missing[0] != "sendRichMessage" {
		t.Fatalf("missing = %v, want [sendRichMessage]", missing)
	}
}

// Preflight is how a bot refuses to start on a server that cannot serve it. A
// probe the server could not answer must stop it with a message that says so,
// not let it through and not blame a method the server never ruled on.
func TestPreflightFailsClearlyWhenAProbeIsInconclusive(t *testing.T) {
	for _, status := range []int{http.StatusConflict, http.StatusTooManyRequests, http.StatusBadGateway} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/getMe") {
				_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"bot"}}`))
				return
			}
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, `{"ok":false,"error_code":%d,"description":"server says no"}`, status)
		})

		_, err := c.Preflight(context.Background(), Needs{Methods: []string{"sendRichMessage"}})
		if err == nil {
			t.Fatalf("status %d: Preflight passed although the server never answered about sendRichMessage", status)
		}
		var missing *MissingMethodsError
		if errors.As(err, &missing) {
			t.Fatalf("status %d: error = %v, the server did not say the method is missing", status, err)
		}
		if !strings.Contains(err.Error(), "cannot tell whether sendRichMessage exists") {
			t.Fatalf("status %d: message = %q", status, err.Error())
		}
	}
}

// Wait is what an operator sizes a restart policy on, so it has to bound the
// whole startup check rather than only the pauses inside it.
func TestPreflightWaitBoundsTheWholeCheck(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":502,"description":"Bad Gateway"}`))
	})
	// Real sleeps here: the point is the wall clock.
	started := time.Now()
	_, err := c.Preflight(context.Background(), Needs{Wait: 300 * time.Millisecond})
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("want the failure surfaced")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("took %s for a 300ms budget", elapsed)
	}
}

// A probe calls a method with an empty body on purpose and is answered with a
// parameter error. Counted as a failure, that is one phantom incident per
// probed method on every start -- which is exactly what showed up in voicy's
// metrics after a restart.
func TestProbeEventsAreMarked(t *testing.T) {
	var events []Event
	c, _ := newTestClient(t, oldServer(map[string]bool{"sendMessage": true}),
		WithObserver(func(e Event) { events = append(events, e) }))

	if _, err := c.Probe(context.Background(), "sendMessage"); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("the probe made no observable request")
	}
	for _, e := range events {
		if !e.Probe {
			t.Fatalf("event %+v is not marked as a probe", e)
		}
	}

	events = nil
	if _, err := c.SendMessage(context.Background(), 1, "real work", nil); err == nil {
		t.Fatal("the stub refuses everything; expected an error")
	}
	for _, e := range events {
		if e.Probe {
			t.Fatalf("event %+v is work the bot asked for, not a probe", e)
		}
	}
}

// Every method this package sends should be probeable, or a bot cannot refuse
// to start on a server that lacks it -- which is the whole point of Preflight.
func TestMethodsThisPackageSendsAreProbeSafe(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: chat_id is empty"}`))
	})

	// The media and inline surface, which is what searchy cannot run without.
	methods := []string{
		"answerInlineQuery", "editMessageMedia", "editMessageReplyMarkup",
		"sendAudio", "sendDocument", "sendMediaGroup", "sendPhoto", "sendVideo",
	}
	missing, err := c.Probe(context.Background(), methods...)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing = %v, want none: a 400 on an empty body means the method is there", missing)
	}
}

// framyProbeMethods are the methods added for framy that Preflight can now
// require. Each one is in probeSafe because Telegram's server reads its
// mandatory parameter first and answers with a parameter error before it does
// anything: sendAnimation without a file ("There is no animation in the
// request"), editMessageCaption without a message ("Message identifier is not
// specified"), deleteMessages without ids ("Message identifiers are not
// specified"), the reactions without a chat ("chat_id is empty"), the
// ephemeral edits without media ("Parameter \"media\" is required") or a
// receiver ("Invalid receiver_user_id specified").
var framyProbeMethods = []string{
	"deleteMessageReaction", "deleteMessages", "editEphemeralMessageCaption", "editEphemeralMessageMedia",
	"editEphemeralMessageReplyMarkup", "editMessageCaption", "sendAnimation", "setMessageReaction",
}

func TestPreflightCanRequireTheMethodsAddedForFramy(t *testing.T) {
	var bodies []string
	known := map[string]bool{}
	for _, m := range framyProbeMethods {
		known[m] = true
	}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"framyin_bot"}}`))
			return
		}
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]+" "+string(raw))
		oldServer(known)(w, r)
	})

	if _, err := c.Preflight(context.Background(), Needs{Methods: framyProbeMethods}); err != nil {
		t.Fatalf("Preflight on a server that has them: %v", err)
	}
	// Probing is a real call. These methods are only safe to probe because the
	// body is empty, so the test pins that nothing else rides along.
	for _, sent := range bodies {
		if !strings.HasSuffix(sent, " {}") {
			t.Fatalf("probe sent %q, want an empty object", sent)
		}
	}
	if len(bodies) != len(framyProbeMethods) {
		t.Fatalf("probes = %v, want one per method", bodies)
	}

	for _, method := range framyProbeMethods {
		old, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/getMe") {
				_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"framyin_bot"}}`))
				return
			}
			oldServer(map[string]bool{})(w, r)
		})
		_, err := old.Preflight(context.Background(), Needs{Methods: []string{method}})
		var missing *MissingMethodsError
		if !errors.As(err, &missing) || len(missing.Methods) != 1 || missing.Methods[0] != method {
			t.Fatalf("Preflight against a server without %s: err = %v, want it named as missing", method, err)
		}
	}
}
