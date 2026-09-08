// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
	"net/url"
)

// Call sends an arbitrary Bot API method as JSON and decodes the whole
// response envelope into out, which should therefore carry both "ok" and
// "result" fields.
//
// It exists so that a bot needing a method this module has not learned yet is
// never blocked on a release: the call still goes through the same transport,
// retry policy, token redaction and observer. Move the method into methods.go
// once a second bot wants it.
func (c *Client) Call(ctx context.Context, method string, req any, out any) error {
	return c.post(ctx, method, req, out)
}

// CallGet is [Client.Call] for a method that reads. Unlike Call it is retried,
// so use it only for calls that are safe to repeat.
func (c *Client) CallGet(ctx context.Context, method string, values url.Values, out any) error {
	if values == nil {
		values = url.Values{}
	}
	return c.get(ctx, method, values, out)
}

// okFalse is the error for a 2xx answer whose payload says ok=false. Telegram
// does not do this in practice, but a proxy or a stale server can.
func okFalse(method string) error {
	return fmt.Errorf("telegram %s returned ok=false", method)
}
