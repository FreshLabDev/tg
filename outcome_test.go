// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strings"
	"testing"
)

// An answer from Telegram settles the question: a refusal did nothing, a 5xx
// may have done it.
func TestOutcomeOfAnAnswer(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{400, `{"ok":false,"error_code":400,"description":"Bad Request: chat not found"}`, false},
		{403, `{"ok":false,"error_code":403,"description":"Forbidden: bot was blocked by the user"}`, false},
		{429, `{"ok":false,"error_code":429,"description":"Too Many Requests: retry after 3600","parameters":{"retry_after":3600}}`, false},
		{500, `{"ok":false,"error_code":500,"description":"Internal Server Error"}`, true},
		{502, `<html>Bad Gateway</html>`, true},
		// A proxy that wraps a failure in a 200: the error_code is what counts.
		{200, `{"ok":false,"error_code":502,"description":"Bad Gateway"}`, true},
	} {
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})
		_, err := c.SendMessage(context.Background(), 1, "hi", nil)
		if err == nil {
			t.Fatalf("%d: want an error", tc.status)
		}
		if got := OutcomeUnknown(err); got != tc.want {
			t.Fatalf("%d %s: OutcomeUnknown = %v, want %v (err %v)", tc.status, tc.body, got, tc.want, err)
		}
	}
}

// A 2xx whose result cannot be read was accepted: the message is there.
func TestOutcomeOfAnUnreadableResultIsUnknown(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":"not a message"}`))
	})
	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if !errors.Is(err, ErrUnexpectedResult) || !OutcomeUnknown(err) {
		t.Fatalf("err = %v, OutcomeUnknown = %v, want an accepted call", err, OutcomeUnknown(err))
	}
}

// The connection drops after the server has read the whole request: it may
// have acted on it.
func TestOutcomeOfAConnectionLostAfterTheRequestIsUnknown(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		_ = conn.Close()
	})
	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if err == nil || !OutcomeUnknown(err) {
		t.Fatalf("err = %v, OutcomeUnknown = %v, want true", err, OutcomeUnknown(err))
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatal("the error carries the token")
	}
}

// Nothing listens on the port: the request never left.
func TestOutcomeOfARefusedConnectionIsKnown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	c := New(testToken, WithAPIBase(base))
	_, err := c.SendMessage(context.Background(), 1, "hi", nil)
	if err == nil {
		t.Fatal("want an error from a closed port")
	}
	if OutcomeUnknown(err) {
		t.Fatalf("err = %v: a connection that was refused delivered nothing", err)
	}
}

// reporting is a RoundTripper that reports what a real transport would and
// then fails, so a test can stage a failure at a chosen point.
type reporting struct {
	connect bool
	wrote   []error // one WroteRequest per entry
	fail    error
}

func (rt reporting) RoundTrip(req *http.Request) (*http.Response, error) {
	trace := httptrace.ContextClientTrace(req.Context())
	if trace != nil && rt.connect {
		trace.GetConn(req.URL.Host)
	}
	for _, err := range rt.wrote {
		if trace != nil {
			trace.WroteRequest(httptrace.WroteRequestInfo{Err: err})
		}
	}
	return nil, rt.fail
}

func TestOutcomeFollowsHowMuchOfTheRequestWasWritten(t *testing.T) {
	broken := errors.New("connection reset by peer")
	for name, tc := range map[string]struct {
		rt   reporting
		want bool
	}{
		// An upload that broke off halfway: the server never had the request.
		"cut off while writing": {reporting{connect: true, wrote: []error{io.ErrUnexpectedEOF}, fail: broken}, false},
		"no connection":         {reporting{connect: true, fail: broken}, false},
		"written, then lost":    {reporting{connect: true, wrote: []error{nil}, fail: broken}, true},
		// A transport that retried after the server had the first attempt.
		"written once, then cut off": {reporting{connect: true, wrote: []error{nil, io.ErrUnexpectedEOF}, fail: broken}, true},
		// A transport that reports nothing proves nothing.
		"silent transport": {reporting{fail: broken}, true},
	} {
		c := New(testToken, WithAPIBase("https://bot.example.test"), WithHTTPClient(&http.Client{Transport: tc.rt}))
		_, err := c.SendMessage(context.Background(), 1, "hi", nil)
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if got := OutcomeUnknown(err); got != tc.want {
			t.Fatalf("%s: OutcomeUnknown = %v, want %v", name, got, tc.want)
		}
	}
}

// A call refused before any request, a caller that had already given up, and
// no error at all did nothing.
func TestOutcomeOfNothingSentIsKnown(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("nothing should reach the server")
	})
	if _, err := c.SendMediaGroup(context.Background(), 1, nil, MediaGroupOptions{}); err == nil || OutcomeUnknown(err) {
		t.Fatalf("an empty album: err = %v, OutcomeUnknown = %v", err, OutcomeUnknown(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.SendMessage(ctx, 1, "hi", nil); err == nil || OutcomeUnknown(err) {
		t.Fatalf("a canceled caller: err = %v, OutcomeUnknown = %v", err, OutcomeUnknown(err))
	}
	if OutcomeUnknown(nil) || OutcomeUnknown(fmt.Errorf("some error of the bot's own")) {
		t.Fatal("an error that is not a call's outcome reads as unknown")
	}
}
