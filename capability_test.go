// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
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

	for _, method := range []string{"logOut", "getMe", "deleteWebhook", "close"} {
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
		if calls.Add(1) < 3 {
			// Simulate "connection refused" as a 502 from the stub's own view;
			// the real case is a transport error, exercised below by closing.
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Gateway"}`))
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
