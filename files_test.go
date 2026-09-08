// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
