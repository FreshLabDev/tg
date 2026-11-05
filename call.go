// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
	"net/url"
	"sort"
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

// CallMultipart is [Client.Call] for a method this package does not model that
// carries files. Each entry of files is the top-level parameter of its name: a
// file_id, a URL or a path on a --local server goes as that parameter's value,
// and an upload as a multipart part of the same name. Nothing is uploaded, and
// the request stays JSON, unless one of them is an [InputFileUpload].
//
// A file inside an object -- an InputMedia of a method that is not modeled --
// is referred to from its field as "attach://<name>", which works only for an
// upload; a file_id or a URL goes into such a field as itself.
//
// The call has the upload timeout when it carries a file, and the retries and
// redaction of every other POST. fields is not modified, and a name in both
// fields and files is refused before sending.
func (c *Client) CallMultipart(ctx context.Context, method string, fields map[string]any, files map[string]InputFile, out any) error {
	merged := make(map[string]any, len(fields)+len(files))
	for name, value := range fields {
		merged[name] = value
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var parts []attachment
	for _, name := range names {
		if _, taken := merged[name]; taken {
			return fmt.Errorf("%s: %q is both a field and a file", method, name)
		}
		if err := c.attachTop(merged, &parts, name, files[name]); err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
	}
	return c.submit(ctx, method, merged, parts, out)
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
