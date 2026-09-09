// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// uploaded is one multipart request, decoded the way Telegram reads it.
type uploaded struct {
	method string
	fields map[string]string
	files  map[string]string // part name -> contents
	names  map[string]string // part name -> filename
}

// captureUpload returns a stub that decodes a multipart body, so a test can
// assert on the parts Telegram would see rather than on this package's
// encoding of them.
func captureUpload(t *testing.T, result string) (http.HandlerFunc, *uploaded) {
	t.Helper()
	got := &uploaded{fields: map[string]string{}, files: map[string]string{}, names: map[string]string{}}
	return func(w http.ResponseWriter, r *http.Request) {
		got.method = r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		contentType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "multipart/form-data" {
			t.Errorf("Content-Type = %q, want multipart/form-data", r.Header.Get("Content-Type"))
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("read part: %v", err)
				return
			}
			raw, _ := io.ReadAll(part)
			if part.FileName() != "" {
				got.files[part.FormName()] = string(raw)
				got.names[part.FormName()] = part.FileName()
				continue
			}
			got.fields[part.FormName()] = string(raw)
		}
		_, _ = w.Write([]byte(result))
	}, got
}

// A file_id costs no upload, so the request stays the JSON one every other
// method sends.
func TestSendPhotoByFileIDStaysJSON(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":9}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendPhoto(context.Background(), 42, InputFileString{Data: "AgACfileid"}, CaptionOptions{
		ThreadID: 3,
		ReplyTo:  11,
		Caption:  "<b>cover</b>",
		Markup:   &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "open", URL: "https://example.test"}}}},
	})
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if msg.MessageID != 9 {
		t.Fatalf("message_id = %d", msg.MessageID)
	}
	if *method != "sendPhoto" {
		t.Fatalf("method = %q", *method)
	}
	if (*body)["photo"] != "AgACfileid" {
		t.Fatalf("photo = %v, want the identifier as a plain parameter", (*body)["photo"])
	}
	if (*body)["parse_mode"] != "HTML" {
		t.Fatal("a caption is HTML like every other text this package sends")
	}
	if (*body)["message_thread_id"] != float64(3) {
		t.Fatalf("message_thread_id = %v", (*body)["message_thread_id"])
	}
	reply, _ := (*body)["reply_parameters"].(map[string]any)
	if reply["allow_sending_without_reply"] != true {
		t.Fatal("a deleted original must not fail the send")
	}
}

func TestSendPhotoOmitsCaptionParseModeWhenThereIsNoCaption(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendPhoto(context.Background(), 42, InputFileString{Data: "id"}, CaptionOptions{}); err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	for _, absent := range []string{"caption", "parse_mode", "message_thread_id", "reply_parameters", "reply_markup"} {
		if _, has := (*body)[absent]; has {
			t.Fatalf("%s must be absent when it was not asked for", absent)
		}
	}
}

// Bytes are the only case that needs a different body, and the file becomes
// the part named after the parameter itself.
func TestSendPhotoUploadsBytesAsMultipart(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":4}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendPhoto(context.Background(), 42, InputFileUpload{Filename: "grid.jpg", Data: bytes.NewReader([]byte("jpegbytes"))}, CaptionOptions{
		Caption: "page 1",
	})
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if msg.MessageID != 4 {
		t.Fatalf("message_id = %d", msg.MessageID)
	}
	if got.method != "sendPhoto" {
		t.Fatalf("method = %q", got.method)
	}
	if got.files["photo"] != "jpegbytes" {
		t.Fatalf("photo part = %q", got.files["photo"])
	}
	if got.names["photo"] != "grid.jpg" {
		t.Fatalf("filename = %q, want the caller's", got.names["photo"])
	}
	if got.fields["chat_id"] != "42" || got.fields["caption"] != "page 1" || got.fields["parse_mode"] != "HTML" {
		t.Fatalf("scalar fields = %v", got.fields)
	}
	if _, has := got.fields["photo"]; has {
		t.Fatal("an uploaded file must not also travel as a string parameter")
	}
}

func TestUploadWithoutAFilenameStillNamesThePart(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendDocument(context.Background(), 1, InputFileUpload{Data: strings.NewReader("x")}, DocumentOptions{}); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	if got.names["document"] == "" {
		t.Fatal("a part with no filename is not read as a file at all")
	}
}

// reply_markup is an object, and it still has to arrive as JSON once the body
// itself has stopped being JSON.
func TestMultipartEncodesObjectsAsJSONAndScalarsAsText(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendVideo(context.Background(), 42, InputFileUpload{Filename: "clip.mp4", Data: strings.NewReader("mp4")}, VideoOptions{
		CaptionOptions:    CaptionOptions{Markup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "go", CallbackData: "d"}}}}},
		Width:             640,
		Height:            480,
		Duration:          12,
		SupportsStreaming: true,
	})
	if err != nil {
		t.Fatalf("SendVideo: %v", err)
	}
	if got.fields["width"] != "640" || got.fields["height"] != "480" || got.fields["duration"] != "12" {
		t.Fatalf("dimensions = %v", got.fields)
	}
	if got.fields["supports_streaming"] != "true" {
		t.Fatalf("supports_streaming = %q, want a bare boolean", got.fields["supports_streaming"])
	}
	var markup InlineKeyboardMarkup
	if err := json.Unmarshal([]byte(got.fields["reply_markup"]), &markup); err != nil {
		t.Fatalf("reply_markup is not JSON: %v", err)
	}
	if markup.InlineKeyboard[0][0].Text != "go" {
		t.Fatalf("reply_markup = %q", got.fields["reply_markup"])
	}
}

func TestSendAudioCarriesTrackMetadata(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendAudio(context.Background(), 42, InputFileString{Data: "id"}, AudioOptions{
		Duration: 90, Performer: "Someone", Title: "A track",
	})
	if err != nil {
		t.Fatalf("SendAudio: %v", err)
	}
	if *method != "sendAudio" {
		t.Fatalf("method = %q", *method)
	}
	if (*body)["performer"] != "Someone" || (*body)["title"] != "A track" || (*body)["duration"] != float64(90) {
		t.Fatalf("body = %v", *body)
	}
}

func TestSendDocumentKeepsTelegramFromReinterpretingTheFile(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendDocument(context.Background(), 42, InputFileString{Data: "id"}, DocumentOptions{
		DisableContentTypeDetection: true,
	})
	if err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	if (*body)["disable_content_type_detection"] != true {
		t.Fatal("a document sent deliberately must not come back as a photo")
	}
}

// A media group mixes files Telegram already has with files it does not, and
// only the second kind is uploaded -- each under its own attach:// name.
func TestSendMediaGroupMixesIdentifiersAndUploads(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	messages, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaPhoto{Media: InputFileString{Data: "known-id"}, Caption: "first"},
		InputMediaVideo{Media: InputFileUpload{Filename: "clip.mp4", Data: strings.NewReader("mp4")}, SupportsStreaming: true},
	}, 5)
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	if len(messages) != 2 || messages[1].MessageID != 2 {
		t.Fatalf("messages = %v", messages)
	}
	if got.method != "sendMediaGroup" {
		t.Fatalf("method = %q", got.method)
	}
	if got.fields["message_thread_id"] != "5" {
		t.Fatalf("message_thread_id = %q", got.fields["message_thread_id"])
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &items); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if items[0]["media"] != "known-id" || items[0]["type"] != "photo" || items[0]["caption"] != "first" {
		t.Fatalf("first item = %v", items[0])
	}
	if items[1]["media"] != "attach://media1" || items[1]["type"] != "video" {
		t.Fatalf("second item = %v", items[1])
	}
	if got.files["media1"] != "mp4" {
		t.Fatalf("uploaded part = %q", got.files["media1"])
	}
	if _, has := got.files["media0"]; has {
		t.Fatal("a file Telegram already holds must not be uploaded again")
	}
}

func TestSendMediaGroupRefusesAnEmptyAlbum(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an empty album must not reach the network")
	})
	if _, err := c.SendMediaGroup(context.Background(), 1, nil, 0); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestEditMessageMediaUploadsTheReplacement(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	err := c.EditMessageMedia(context.Background(), 42, 7,
		InputMediaPhoto{Media: InputFileUpload{Filename: "page2.jpg", Data: strings.NewReader("jpeg")}, Caption: "page 2"},
		&InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "next", CallbackData: "n"}}}})
	if err != nil {
		t.Fatalf("EditMessageMedia: %v", err)
	}
	if got.method != "editMessageMedia" {
		t.Fatalf("method = %q", got.method)
	}
	var media map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if media["media"] != "attach://media" {
		t.Fatalf("media = %v", media)
	}
	if media["parse_mode"] != "HTML" {
		t.Fatal("a caption is HTML here too")
	}
	if got.files["media"] != "jpeg" {
		t.Fatalf("uploaded part = %q", got.files["media"])
	}
	if got.fields["message_id"] != "7" {
		t.Fatalf("message_id = %q", got.fields["message_id"])
	}
}

// Re-rendering the same page is what a double-tap produces, and it must not
// look like a failure to the caller.
func TestEditMessageMediaTreatsNotModifiedAsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: message is not modified"}`))
	})
	err := c.EditMessageMedia(context.Background(), 1, 2, InputMediaPhoto{Media: InputFileString{Data: "id"}}, nil)
	if err != nil {
		t.Fatalf("EditMessageMedia: %v", err)
	}
}

func TestEditMessageReplyMarkupSendsOnlyTheButtons(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	err := c.EditMessageReplyMarkup(context.Background(), 42, 7,
		&InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "open", URL: "https://example.test"}}}})
	if err != nil {
		t.Fatalf("EditMessageReplyMarkup: %v", err)
	}
	if *method != "editMessageReplyMarkup" {
		t.Fatalf("method = %q", *method)
	}
	if _, has := (*body)["text"]; has {
		t.Fatal("changing the buttons must not touch the content the user is reading")
	}
	if _, has := (*body)["reply_markup"]; !has {
		t.Fatal("reply_markup is missing")
	}
}

func TestEditMessageReplyMarkupTreatsNotModifiedAsSuccess(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"description":"Bad Request: message is not modified"}`))
	})
	if err := c.EditMessageReplyMarkup(context.Background(), 1, 2, nil); err != nil {
		t.Fatalf("EditMessageReplyMarkup: %v", err)
	}
}

// A path on the server's disk is only meaningful to a server that has one.
// Sending it to Telegram's endpoint would fail with a description about an
// unsupported URL, which says nothing about the actual mistake.
func TestLocalPathIsRefusedAgainstTelegramsOwnServer(t *testing.T) {
	c := New(testToken)
	_, err := c.SendVideo(context.Background(), 1, InputFileLocal{Path: "/srv/cache/clip.mp4"}, VideoOptions{})
	if err == nil {
		t.Fatal("want a refusal naming the server")
	}
	if !strings.Contains(err.Error(), "self-hosted") {
		t.Fatalf("message = %q", err.Error())
	}
}

func TestLocalPathBecomesAFileURIOnASelfHostedServer(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendVideo(context.Background(), 1, InputFileLocal{Path: "/srv/cache/clip.mp4"}, VideoOptions{}); err != nil {
		t.Fatalf("SendVideo: %v", err)
	}
	if (*body)["video"] != "file:///srv/cache/clip.mp4" {
		t.Fatalf("video = %v", (*body)["video"])
	}
}

func TestRelativeLocalPathIsRefused(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a path this process cannot even name must not reach the network")
	})
	if _, err := c.SendVideo(context.Background(), 1, InputFileLocal{Path: "cache/clip.mp4"}, VideoOptions{}); err == nil {
		t.Fatal("want a refusal")
	}
}

func TestSendMediaRejectsAnEmptyIdentifierBeforeSending(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an empty identifier must not reach the network")
	})
	if _, err := c.SendPhoto(context.Background(), 1, InputFileString{}, CaptionOptions{}); err == nil {
		t.Fatal("want a refusal")
	}
	if _, err := c.SendPhoto(context.Background(), 1, nil, CaptionOptions{}); err == nil {
		t.Fatal("want a refusal for a missing file")
	}
}
