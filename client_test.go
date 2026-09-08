// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const testToken = "123456:AAtest-token-value"

// newTestClient points a client at a stub server and makes backoff free, so a
// retry test costs no wall-clock time.
func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	opts = append([]Option{WithAPIBase(srv.URL), WithHTTPClient(srv.Client())}, opts...)
	c := New(testToken, opts...)
	c.sleep = func(ctx context.Context, d time.Duration) error { return ctx.Err() }
	return c, srv
}

func TestEndpointCarriesTokenAndMethod(t *testing.T) {
	c := New(testToken, WithAPIBase("https://example.test/"))
	got := c.endpoint("getMe")
	want := "https://example.test/bot" + testToken + "/getMe"
	if got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestDefaultAPIBaseIsTelegram(t *testing.T) {
	c := New(testToken)
	if c.APIBase() != DefaultAPIBase {
		t.Fatalf("APIBase = %q, want %q", c.APIBase(), DefaultAPIBase)
	}
	if c.IsLocalServer() {
		t.Fatal("a default client must not look like a self-hosted one")
	}
}
