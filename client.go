// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

// DefaultAPIBase is Telegram's own server. A self-hosted one is set with
// [WithAPIBase].
const DefaultAPIBase = "https://api.telegram.org"

// Client talks to one bot's Bot API endpoint. It is safe for concurrent use.
type Client struct {
	token     string
	apiBase   string
	http      *http.Client
	timeout   time.Duration
	sleep     func(context.Context, time.Duration) error
	log       *slog.Logger
	observe   Observer
	filesRoot string
	allowed   []string
}

// Event describes one HTTP attempt against the Bot API. It carries no bot
// token and no response body, so it is safe to log or turn into metrics.
type Event struct {
	// Method is the Bot API method name, e.g. "sendMessage".
	Method string
	// Status is the HTTP status, or 0 when the request never got a response.
	Status int
	// Duration covers the request and reading the response body.
	Duration time.Duration
	// Attempt counts from 1.
	Attempt int
	// Retried reports whether another attempt will follow this one.
	Retried bool
	// Err is the attempt's error, already redacted of the bot token.
	Err error
}

// Observer is called once per HTTP attempt. It must not block: it runs on the
// calling goroutine, inside the request path.
type Observer func(Event)

// Option configures a Client.
type Option func(*Client)

// WithAPIBase points the client at a self-hosted Bot API server. An empty
// value keeps [DefaultAPIBase].
func WithAPIBase(base string) Option {
	return func(c *Client) {
		if base != "" {
			c.apiBase = strings.TrimRight(base, "/")
		}
	}
}

// WithHTTPClient replaces the transport. The client must not set a global
// Timeout that is shorter than a long poll: every call carries its own context
// deadline instead.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.http = h
		}
	}
}

// WithTimeout sets the per-attempt deadline for ordinary calls. getUpdates and
// file downloads derive their own, longer deadlines.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithLogger attaches a logger. The client logs nothing above debug level on
// its own: errors are returned, not logged, so the caller decides.
func WithLogger(log *slog.Logger) Option {
	return func(c *Client) {
		if log != nil {
			c.log = log
		}
	}
}

// WithAllowedUpdates restricts long polling to the update kinds the bot
// actually handles. Telegram's own default silently omits some kinds
// (my_chat_member among them), so a bot that needs one must say so. Passing
// nothing leaves the parameter out and keeps Telegram's default.
func WithAllowedUpdates(kinds ...string) Option {
	return func(c *Client) { c.allowed = kinds }
}

// WithObserver registers a per-attempt callback for metrics.
func WithObserver(o Observer) Option {
	return func(c *Client) { c.observe = o }
}

// WithLocalFiles enables reading media from the data directory of a Bot API
// server started with --local. root is the server's working directory as it is
// visible to this process, e.g. "/var/lib/telegram-bot-api"; the client only
// ever reads under root/<token>, which is the one subdirectory that belongs to
// this bot. Mounting the whole directory of a shared server would expose every
// other bot's token, because that is what its subdirectories are named after.
//
// Without this option an absolute file_path is a hard error: a --local server
// does not serve files over HTTP at all, so there is nothing to fall back to.
func WithLocalFiles(root string) Option {
	return func(c *Client) { c.filesRoot = strings.TrimRight(root, "/") }
}

// New returns a client for token. It performs no I/O; call [Client.Preflight]
// at startup to find out whether the server on the other end can serve it.
func New(token string, opts ...Option) *Client {
	c := &Client{
		token:   token,
		apiBase: DefaultAPIBase,
		http:    defaultHTTPClient(),
		timeout: 30 * time.Second,
		sleep:   sleep,
		log:     slog.New(slog.DiscardHandler),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// APIBase reports the server this client talks to.
func (c *Client) APIBase() string { return c.apiBase }

// IsLocalServer reports whether the client is pointed at something other than
// Telegram's own endpoint.
func (c *Client) IsLocalServer() bool { return c.apiBase != DefaultAPIBase }

// defaultHTTPClient has no global timeout — every attempt carries a context
// deadline — and raises MaxIdleConnsPerHost, whose default of 2 would
// serialize a pool of update workers against a single host.
func defaultHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          64,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}}
}
