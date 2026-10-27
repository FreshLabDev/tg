// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// downloadTimeout bounds a single file transfer. A self-hosted server can hand
// over hundreds of megabytes, so this is generous by design.
const downloadTimeout = 30 * time.Minute

func (c *Client) GetFile(ctx context.Context, fileID string) (File, error) {
	var resp struct {
		OK     bool `json:"ok"`
		Result File `json:"result"`
	}
	if err := c.get(ctx, "getFile", url.Values{"file_id": {fileID}}, &resp); err != nil {
		return File{}, err
	}
	if !resp.OK || resp.Result.FilePath == "" {
		return File{}, fmt.Errorf("telegram getFile returned empty path")
	}
	return resp.Result, nil
}

// Event methods for a download. Neither is a Bot API method: a download is the
// file route, or a read from the disk of a --local server, and the observer
// needs to tell the two apart because only the first crosses a network.
const (
	downloadMethod  = "downloadFile"
	localReadMethod = "readLocalFile"
)

// DownloadToFile streams the media behind a getFile result into dst. Nothing
// is buffered in memory: a single file can be hundreds of megabytes and
// several downloads can run at once.
//
// The shape of filePath decides where the bytes come from. Telegram's own
// server returns a relative path and serves it over HTTPS. A server started
// with --local returns an absolute path on its own disk and serves nothing
// over HTTP at all — its /file/bot<token>/… route answers 404 — so that case
// reads from disk and fails loudly when the directory is not mounted, instead
// of pretending a download could work.
//
// Every download reports one [Event] to the [Observer], successful or not, once
// it has finished: Method is "downloadFile" for the HTTP route and
// "readLocalFile" for a read from disk, Duration covers the whole transfer, and
// Bytes is how much was written to dst. See [Event] for what the fields mean
// here. A limit that is not positive is refused before anything starts and
// reports nothing.
func (c *Client) DownloadToFile(ctx context.Context, filePath, dst string, maxBytes int64) error {
	if maxBytes <= 0 {
		return fmt.Errorf("download limit must be positive")
	}
	started := time.Now()
	if strings.HasPrefix(filePath, "/") {
		written, err := c.copyLocalFile(filePath, dst, maxBytes)
		// Nothing about a file on disk gets better by asking again.
		c.emitDownload(ctx, localReadMethod, 0, started, written, err, false)
		return err
	}
	status, written, err := c.downloadHTTP(ctx, filePath, dst, maxBytes)
	c.emitDownload(ctx, downloadMethod, status, started, written, err, downloadRetryable(err))
	return err
}

// downloadHTTP fetches filePath from the server's file route. It reports the
// HTTP status, 0 when no answer came, and the bytes written to dst.
func (c *Client) downloadHTTP(ctx context.Context, filePath, dst string, maxBytes int64) (int, int64, error) {
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	rawURL := strings.TrimRight(c.apiBase, "/") + "/file/bot" + c.token + "/" + strings.TrimPrefix(filePath, "/")
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, 0, c.redactError(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, c.redactError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, 0, c.redactAPI(parseAPIError(downloadMethod, resp.StatusCode, resp.Body))
	}
	written, err := writeLimited(dst, resp.Body, maxBytes)
	return resp.StatusCode, written, err
}

// downloadRetryable says whether a failed download is of a kind that could pass
// on another try: the network, a 429, a 5xx. This package never retries one;
// the answer is for the observer. A file over the limit, a file missing on disk
// and a target that cannot be written fail the same way next time.
func downloadRetryable(err error) bool {
	if err == nil || errors.Is(err, ErrFileTooLarge) {
		return false
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return false
	}
	return retryableError(err)
}

// copyLocalFile copies a file produced by a --local server whose data
// directory is mounted here, then removes the original: a local server keeps
// every file it ever produced and never reclaims the space. It reports the bytes
// written to dst.
func (c *Client) copyLocalFile(path, dst string, maxBytes int64) (int64, error) {
	if c.filesRoot == "" {
		return 0, fmt.Errorf("bot api server returned an absolute file path %q: it runs with --local and serves no files over HTTP, so its data directory must be mounted and WithLocalFiles set", c.redact(path))
	}
	clean, err := c.resolveUnderBotDir(path)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(clean)
	if err != nil {
		return 0, fmt.Errorf("local bot api file unavailable: %w", c.redactPathError(err))
	}
	if info.Size() > maxBytes {
		return 0, fmt.Errorf("%w: %d bytes", ErrFileTooLarge, info.Size())
	}
	in, err := os.Open(clean)
	if err != nil {
		return 0, fmt.Errorf("open local bot api file: %w", c.redactPathError(err))
	}
	defer in.Close()
	written, err := writeLimited(dst, in, maxBytes)
	if err != nil {
		return written, err
	}
	_ = os.Remove(clean)
	return written, nil
}

// resolveUnderBotDir turns a path the server handed over into a real path
// inside this bot's own directory, or refuses it.
//
// Cleaning the path stops a "..", but not a symlink: the directory is written
// by another process on a volume shared with every other bot, so a link
// planted there would otherwise be followed straight out of the boundary this
// check exists to hold. Both sides are resolved before they are compared.
func (c *Client) resolveUnderBotDir(path string) (string, error) {
	base := filepath.Clean(filepath.Join(c.filesRoot, c.token))
	// A directory that is not there yet cannot be resolved, and cannot contain
	// anything either: the lexical form is enough to refuse the path below.
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	clean, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("bot api file unavailable: %w", c.redactPathError(err))
	}
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return "", fmt.Errorf("bot api file %q resolves outside this bot's directory %q: a shared server's data directory holds every other bot's files", c.redact(clean), c.redact(base))
	}
	return clean, nil
}

// redactPathError keeps the error's identity -- fs.ErrNotExist and friends
// still match through errors.Is -- while scrubbing the token out of the path
// it carries. A local server names each bot's directory after its token, so
// the path is as sensitive as a URL.
func (c *Client) redactPathError(err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return &fs.PathError{Op: pathErr.Op, Path: c.redact(pathErr.Path), Err: pathErr.Err}
	}
	return c.redactError(err)
}

// VerifyLocalFiles checks at startup what a download would otherwise discover
// on the first voice message: that the server's data directory for this bot is
// mounted, readable and writable. Files are deleted after use, so write access
// to the directory is part of the contract.
func (c *Client) VerifyLocalFiles() error {
	if c.filesRoot == "" {
		return fmt.Errorf("no local files directory configured")
	}
	dir := filepath.Join(c.filesRoot, c.token)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("bot api files directory %q is not available: %w", c.redact(dir), c.redactPathError(err))
	}
	if !info.IsDir() {
		return fmt.Errorf("bot api files path %q is not a directory", c.redact(dir))
	}
	probe, err := os.CreateTemp(dir, ".tg-preflight-*")
	if err != nil {
		return fmt.Errorf("bot api files directory %q is not writable, so downloaded files could never be removed: %w", c.redact(dir), c.redactPathError(err))
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return nil
}

// redact removes the bot token from a string that is about to be shown to a
// human. A local server names every bot's directory after its token, so paths
// are as sensitive as URLs.
func (c *Client) redact(s string) string {
	if c.token == "" {
		return s
	}
	return strings.ReplaceAll(s, c.token, "***")
}

// writeLimited copies at most maxBytes into dst and fails if the source has
// more, so an oversize file is rejected without ever being held whole. A
// failure removes dst: os.Create truncates before the size is known, and a
// caller that finds a file where an error was reported will process it. It
// returns how many bytes were written, including those of a file that was then
// removed.
func writeLimited(dst string, src io.Reader, maxBytes int64) (written int64, err error) {
	out, err := os.Create(dst)
	if err != nil {
		return 0, fmt.Errorf("create download target: %w", err)
	}
	defer func() {
		closeErr := out.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(dst)
		}
	}()
	copied, err := io.Copy(out, io.LimitReader(src, maxBytes+1))
	written = copied
	if err != nil {
		return written, fmt.Errorf("write download: %w", err)
	}
	if written > maxBytes {
		return written, fmt.Errorf("%w: more than %d bytes", ErrFileTooLarge, maxBytes)
	}
	return written, out.Sync()
}
