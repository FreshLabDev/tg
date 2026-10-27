// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// localServerDir builds the directory layout a --local server produces:
// <root>/<token>/<kind>/<file>.
func localServerDir(t *testing.T, name string, content []byte) (root, path string) {
	t.Helper()
	root = t.TempDir()
	dir := filepath.Join(root, testToken, "video_notes")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o640); err != nil {
		t.Fatal(err)
	}
	return root, path
}

func TestDownloadReadsFromDiskAndReclaimsTheFile(t *testing.T) {
	root, path := localServerDir(t, "file_0.mp4", []byte("circle"))
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no HTTP request may be made for an absolute path, got %s", r.URL.Path)
	}, WithLocalFiles(root))

	dst := filepath.Join(t.TempDir(), "out.mp4")
	if err := c.DownloadToFile(context.Background(), path, dst, 1<<20); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "circle" {
		t.Fatalf("copied = %q, err = %v", got, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a local server never reclaims files, so the client must remove the original")
	}
}

// This is the bug that made voicy go silent: the client tried to fetch the
// file over HTTP from a server running with --local, which serves none.
func TestAbsolutePathWithoutLocalFilesIsAClearError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a --local path must not be attempted over HTTP, got %s", r.URL.Path)
	})

	err := c.DownloadToFile(context.Background(), "/var/lib/telegram-bot-api/"+testToken+"/video_notes/file_0.mp4",
		filepath.Join(t.TempDir(), "out"), 1<<20)
	if err == nil {
		t.Fatal("want an error naming the missing configuration")
	}
	if !strings.Contains(err.Error(), "--local") || !strings.Contains(err.Error(), "WithLocalFiles") {
		t.Fatalf("message = %q, want it to say what to configure", err.Error())
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("the token leaked through a path: %q", err.Error())
	}
}

// The data directory of a shared server holds one subdirectory per bot, named
// after that bot's token. Nothing outside this bot's own subtree is ours.
func TestDownloadRefusesPathsOutsideThisBotsDirectory(t *testing.T) {
	root := t.TempDir()
	other := filepath.Join(root, "999:OTHERBOT")
	if err := os.MkdirAll(other, 0o750); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(other, "td.binlog")
	if err := os.WriteFile(secret, []byte("someone else's session"), 0o640); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClient(t, nil, WithLocalFiles(root))

	err := c.DownloadToFile(context.Background(), secret, filepath.Join(t.TempDir(), "out"), 1<<20)
	if err == nil {
		t.Fatal("want a refusal for a path belonging to another bot")
	}
	if !strings.Contains(err.Error(), "outside this bot's directory") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestDownloadRejectsAnOversizeLocalFile(t *testing.T) {
	root, path := localServerDir(t, "file_1.mp4", []byte("0123456789"))
	c, _ := newTestClient(t, nil, WithLocalFiles(root))

	err := c.DownloadToFile(context.Background(), path, filepath.Join(t.TempDir(), "out"), 4)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatal("a file rejected as oversize must be left alone, not consumed")
	}
}

func TestDownloadOverHTTPForTelegramsOwnServer(t *testing.T) {
	var path string
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte("voice bytes"))
	})

	dst := filepath.Join(t.TempDir(), "out.oga")
	if err := c.DownloadToFile(context.Background(), "voice/file_1.oga", dst, 1<<20); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	if want := "/file/bot" + testToken + "/voice/file_1.oga"; path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "voice bytes" {
		t.Fatalf("downloaded = %q", got)
	}
}

func TestDownloadOverHTTPRejectsOversize(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	})

	err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 4)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
}

func TestVerifyLocalFilesChecksTheMount(t *testing.T) {
	root, _ := localServerDir(t, "file_0.mp4", []byte("x"))
	c := New(testToken, WithLocalFiles(root))
	if err := c.VerifyLocalFiles(); err != nil {
		t.Fatalf("VerifyLocalFiles on a mounted directory: %v", err)
	}

	missing := New(testToken, WithLocalFiles(t.TempDir()))
	err := missing.VerifyLocalFiles()
	if err == nil {
		t.Fatal("want an error when this bot's directory is not there")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("the token leaked: %q", err.Error())
	}
}

func TestGetFileRejectsAnEmptyPath(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"result":{"file_id":"x"}}`))
	})
	if _, err := c.GetFile(context.Background(), "x"); err == nil {
		t.Fatal("want an error for a result without file_path")
	}
}

// The directory is written by another process on a volume shared with every
// other bot. Cleaning the path stops a "..", and nothing else stops a link.
func TestDownloadRefusesASymlinkOutOfTheBotDirectory(t *testing.T) {
	root, _ := localServerDir(t, "file_0.mp4", []byte("ours"))
	outside := filepath.Join(t.TempDir(), "someone-elses-session")
	if err := os.WriteFile(outside, []byte("another bot's td.binlog"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, testToken, "video_notes", "escape.mp4")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	c, _ := newTestClient(t, nil, WithLocalFiles(root))

	dst := filepath.Join(t.TempDir(), "out")
	err := c.DownloadToFile(context.Background(), link, dst, 1<<20)
	if err == nil {
		t.Fatal("a symlink leading out of the bot's directory must be refused")
	}
	if !strings.Contains(err.Error(), "outside this bot's directory") {
		t.Fatalf("message = %q", err.Error())
	}
	if _, statErr := os.Stat(dst); statErr == nil {
		t.Fatal("nothing may be written when the path is refused")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Fatal("the target must be left alone")
	}
}

// A caller that finds a file where an error was reported will process it.
func TestARejectedDownloadLeavesNoFile(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 5000)))
	})

	dst := filepath.Join(t.TempDir(), "out.oga")
	if err := c.DownloadToFile(context.Background(), "voice/file_1.oga", dst, 100); !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a truncated file was left at dst (stat err = %v)", err)
	}
}

// fs.PathError carries the path, and on a local server the path is the token.
func TestLocalFileErrorsKeepTheirKindWithoutTheToken(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, testToken), 0o750); err != nil {
		t.Fatal(err)
	}
	c, _ := newTestClient(t, nil, WithLocalFiles(root))

	err := c.DownloadToFile(context.Background(),
		filepath.Join(root, testToken, "voice", "missing.oga"),
		filepath.Join(t.TempDir(), "out"), 1<<20)
	if err == nil {
		t.Fatal("want an error for a missing file")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("token leaked: %q", err.Error())
	}
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && strings.Contains(pathErr.Path, testToken) {
		t.Fatalf("token leaked through the wrapped cause: %q", pathErr.Path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("the error must still say what kind of failure it was")
	}
}

// A proxy in front of a self-hosted server may echo the request URI, token
// included, in the body of its error page.
func TestDownloadErrorBodyIsRedacted(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no route for " + r.URL.Path))
	})

	err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 1<<20)
	if err == nil {
		t.Fatal("want an error for a 404")
	}
	if strings.Contains(err.Error(), testToken) {
		t.Fatalf("the token is in the error: %v", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("err = %v, want an *APIError with status 404", err)
	}
}

// watch records what the observer is told. Downloads report on the calling
// goroutine, so the slice needs no lock.
func watch() (Option, *[]Event) {
	var seen []Event
	return WithObserver(func(e Event) { seen = append(seen, e) }), &seen
}

// noToken fails if the bot token is anywhere an observer could print it: the
// event itself, or any error in the chain behind it.
func noToken(t *testing.T, e Event) {
	t.Helper()
	if strings.Contains(fmt.Sprintf("%+v", e), testToken) {
		t.Fatalf("the token is in the event: %+v", e)
	}
	for err := e.Err; err != nil; err = errors.Unwrap(err) {
		if strings.Contains(err.Error(), testToken) {
			t.Fatalf("the token is in the event's error chain: %v", err)
		}
	}
}

func TestDownloadReportsOneEventForAnHTTPTransfer(t *testing.T) {
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte("eleven byte"))
	}, observe)

	if err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 1<<20); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("observed %d events, want one for the whole download", len(*seen))
	}
	e := (*seen)[0]
	if e.Method != "downloadFile" || e.Status != http.StatusOK || e.Err != nil || e.Retryable || e.Attempt != 1 || e.Probe {
		t.Fatalf("event = %+v, want a clean first-attempt downloadFile with status 200", e)
	}
	if e.Bytes != 11 {
		t.Fatalf("Bytes = %d, want the 11 written to the file", e.Bytes)
	}
	if e.Duration < 5*time.Millisecond {
		t.Fatalf("Duration = %s, want at least the 5ms the server took", e.Duration)
	}
	noToken(t, e)
}

// The observer is told whether another try could pass, but the package does
// not make it: a download is one attempt.
func TestDownloadEventSaysWhetherAnotherTryCouldPass(t *testing.T) {
	for _, tc := range []struct {
		status    int
		retryable bool
	}{
		{http.StatusNotFound, false},
		{http.StatusTooManyRequests, true},
		{http.StatusBadGateway, true},
	} {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var requests atomic.Int32
			observe, seen := watch()
			c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("no route for " + r.URL.Path))
			}, observe)

			err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 1<<20)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tc.status {
				t.Fatalf("err = %v, want an *APIError with status %d", err, tc.status)
			}
			if requests.Load() != 1 {
				t.Fatalf("%d requests, want one: downloads are not retried", requests.Load())
			}
			if len(*seen) != 1 {
				t.Fatalf("observed %d events, want 1", len(*seen))
			}
			e := (*seen)[0]
			if e.Method != "downloadFile" || e.Status != tc.status || e.Retryable != tc.retryable || e.Bytes != 0 {
				t.Fatalf("event = %+v, want status %d, retryable %v, no bytes", e, tc.status, tc.retryable)
			}
			if e.Err == nil || e.Err.Error() != err.Error() {
				t.Fatalf("event error = %v, want the one DownloadToFile returned (%v)", e.Err, err)
			}
			noToken(t, e)
		})
	}
}

// A file over the limit is rejected after some of it was already written, and
// the bytes are what a dashboard needs to see why a transfer was slow.
func TestDownloadEventForAnOversizeFileCountsWhatWasWritten(t *testing.T) {
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789"))
	}, observe)

	err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 4)
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("err = %v, want ErrFileTooLarge", err)
	}
	e := (*seen)[0]
	if e.Status != http.StatusOK || !errors.Is(e.Err, ErrFileTooLarge) {
		t.Fatalf("event = %+v, want status 200 with ErrFileTooLarge", e)
	}
	if e.Retryable {
		t.Fatal("a file over the limit is over it next time too")
	}
	if e.Bytes <= 0 || e.Bytes > 10 {
		t.Fatalf("Bytes = %d, want what was written before the file was rejected", e.Bytes)
	}
}

// The request URL holds the token, and net/http puts it in the error.
func TestDownloadEventForADroppedConnectionHasNoTokenAndIsRetryable(t *testing.T) {
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}, observe)

	err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 1<<20)
	if err == nil {
		t.Fatal("want an error from a dropped connection")
	}
	if len(*seen) != 1 {
		t.Fatalf("observed %d events, want 1", len(*seen))
	}
	e := (*seen)[0]
	if e.Status != 0 || e.Bytes != 0 || !e.Retryable || e.Err == nil {
		t.Fatalf("event = %+v, want no status, no bytes, a retryable error", e)
	}
	noToken(t, e)
}

func TestDownloadEventForACutTransferKeepsTheBytesThatArrived(t *testing.T) {
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	}, observe)

	dst := filepath.Join(t.TempDir(), "out")
	if err := c.DownloadToFile(context.Background(), "voice/file_1.oga", dst, 1<<20); err == nil {
		t.Fatal("want an error for a body that ends early")
	}
	e := (*seen)[0]
	if e.Status != http.StatusOK || e.Bytes != 5 || !e.Retryable || e.Err == nil {
		t.Fatalf("event = %+v, want status 200, the 5 bytes that arrived, a retryable error", e)
	}
	if _, err := os.Stat(dst); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("half a file was left at dst (stat err = %v)", err)
	}
	noToken(t, e)
}

func TestDownloadEventForALocalReadIsNotAnHTTPRequest(t *testing.T) {
	root, path := localServerDir(t, "file_0.mp4", []byte("circle"))
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a local read made an HTTP request: %s", r.URL.Path)
	}, WithLocalFiles(root), observe)

	if err := c.DownloadToFile(context.Background(), path, filepath.Join(t.TempDir(), "out.mp4"), 1<<20); err != nil {
		t.Fatalf("DownloadToFile: %v", err)
	}
	if len(*seen) != 1 {
		t.Fatalf("observed %d events, want 1", len(*seen))
	}
	e := (*seen)[0]
	if e.Method != "readLocalFile" || e.Status != 0 || e.Err != nil || e.Retryable || e.Attempt != 1 || e.Bytes != 6 {
		t.Fatalf("event = %+v, want a clean readLocalFile of 6 bytes with no status", e)
	}
	noToken(t, e)
}

// The path of a local file is the token, so these are the events that could
// leak it if an error were passed on unredacted.
func TestDownloadEventForAFailedLocalReadKeepsTheErrorAndNoToken(t *testing.T) {
	root, big := localServerDir(t, "big.mp4", []byte("0123456789"))
	missing := filepath.Join(root, testToken, "video_notes", "missing.mp4")
	for _, tc := range []struct {
		name      string
		localRoot string
		path      string
		is        error
	}{
		{"not configured", "", "/var/lib/telegram-bot-api/" + testToken + "/video_notes/file_0.mp4", nil},
		{"missing", root, missing, fs.ErrNotExist},
		{"over the limit", root, big, ErrFileTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observe, seen := watch()
			opts := []Option{observe}
			if tc.localRoot != "" {
				opts = append(opts, WithLocalFiles(tc.localRoot))
			}
			c, _ := newTestClient(t, nil, opts...)

			err := c.DownloadToFile(context.Background(), tc.path, filepath.Join(t.TempDir(), "out"), 4)
			if err == nil {
				t.Fatal("want an error")
			}
			if len(*seen) != 1 {
				t.Fatalf("observed %d events, want 1", len(*seen))
			}
			e := (*seen)[0]
			if e.Method != "readLocalFile" || e.Status != 0 || e.Retryable || e.Bytes != 0 {
				t.Fatalf("event = %+v, want a readLocalFile with no status, no bytes, not retryable", e)
			}
			if e.Err == nil || e.Err.Error() != err.Error() {
				t.Fatalf("event error = %v, want the one DownloadToFile returned (%v)", e.Err, err)
			}
			if tc.is != nil && !errors.Is(e.Err, tc.is) {
				t.Fatalf("event error = %v, want it to keep its kind (%v)", e.Err, tc.is)
			}
			noToken(t, e)
		})
	}
}

func TestRefusedDownloadLimitReportsNothing(t *testing.T) {
	observe, seen := watch()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("a refused limit made a request: %s", r.URL.Path)
	}, observe)

	if err := c.DownloadToFile(context.Background(), "voice/file_1.oga", filepath.Join(t.TempDir(), "out"), 0); err == nil {
		t.Fatal("want an error for a limit of zero")
	}
	if len(*seen) != 0 {
		t.Fatalf("observed %+v, want nothing: no download started", *seen)
	}
}
