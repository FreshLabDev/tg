// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
	"io"
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
func (c *Client) DownloadToFile(ctx context.Context, filePath, dst string, maxBytes int64) error {
	if maxBytes <= 0 {
		return fmt.Errorf("download limit must be positive")
	}
	if strings.HasPrefix(filePath, "/") {
		return c.copyLocalFile(filePath, dst, maxBytes)
	}
	downloadCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	rawURL := strings.TrimRight(c.apiBase, "/") + "/file/bot" + c.token + "/" + strings.TrimPrefix(filePath, "/")
	req, err := http.NewRequestWithContext(downloadCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return c.redactError(err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return c.redactError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError("downloadFile", resp.StatusCode, resp.Body)
	}
	return writeLimited(dst, resp.Body, maxBytes)
}

// copyLocalFile copies a file produced by a --local server whose data
// directory is mounted here, then removes the original: a local server keeps
// every file it ever produced and never reclaims the space.
func (c *Client) copyLocalFile(path, dst string, maxBytes int64) error {
	if c.filesRoot == "" {
		return fmt.Errorf("bot api server returned an absolute file path %q: it runs with --local and serves no files over HTTP, so its data directory must be mounted and WithLocalFiles set", c.redact(path))
	}
	base := filepath.Join(c.filesRoot, c.token)
	clean := filepath.Clean(path)
	if clean != base && !strings.HasPrefix(clean, base+string(filepath.Separator)) {
		return fmt.Errorf("bot api file %q is outside this bot's directory %q: check that WithLocalFiles matches the server's --dir (or --files-dir)", c.redact(clean), c.redact(base))
	}
	info, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("local bot api file unavailable: %w", c.redactError(err))
	}
	if info.Size() > maxBytes {
		return fmt.Errorf("%w: %d bytes", ErrFileTooLarge, info.Size())
	}
	in, err := os.Open(clean)
	if err != nil {
		return fmt.Errorf("open local bot api file: %w", c.redactError(err))
	}
	defer in.Close()
	if err := writeLimited(dst, in, maxBytes); err != nil {
		return err
	}
	_ = os.Remove(clean)
	return nil
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
		return fmt.Errorf("bot api files directory %q is not available: %w", c.redact(dir), c.redactError(err))
	}
	if !info.IsDir() {
		return fmt.Errorf("bot api files path %q is not a directory", c.redact(dir))
	}
	probe, err := os.CreateTemp(dir, ".tg-preflight-*")
	if err != nil {
		return fmt.Errorf("bot api files directory %q is not writable, so downloaded files could never be removed: %w", c.redact(dir), c.redactError(err))
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
// more, so an oversize file is rejected without ever being held whole.
func writeLimited(dst string, src io.Reader, maxBytes int64) error {
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create download target: %w", err)
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return fmt.Errorf("write download: %w", err)
	}
	if written > maxBytes {
		return fmt.Errorf("%w: more than %d bytes", ErrFileTooLarge, maxBytes)
	}
	return out.Sync()
}
