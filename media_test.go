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

// maply sent a photo and then edited its buttons in a second call, because the
// keyboard had no web_app. Both shapes of a send now carry it in the one call.
func TestPhotoWithAWebAppButtonIsOneSend(t *testing.T) {
	markup := &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
		{Text: "Guess", WebApp: &WebAppInfo{URL: "https://maply.example/"}},
		{Text: "Copy", CopyText: &CopyTextButton{Text: "maply #1"}},
	}}}

	t.Run("by file_id", func(t *testing.T) {
		var requests int
		handler, method, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests++
			handler(w, r)
		})
		if _, err := c.SendPhoto(context.Background(), 42, InputFileString{Data: "AgACknown"}, CaptionOptions{Caption: "maply #1", Markup: markup}); err != nil {
			t.Fatalf("SendPhoto: %v", err)
		}
		if requests != 1 || *method != "sendPhoto" {
			t.Fatalf("requests = %d, method = %q, want one sendPhoto", requests, *method)
		}
		raw, _ := json.Marshal((*body)["reply_markup"])
		var sent InlineKeyboardMarkup
		if err := json.Unmarshal(raw, &sent); err != nil || len(sent.InlineKeyboard) != 1 || len(sent.InlineKeyboard[0]) != 2 {
			t.Fatalf("reply_markup = %s (%v)", raw, err)
		}
		if !strings.Contains(string(raw), `"web_app":{"url":"https://maply.example/"}`) || !strings.Contains(string(raw), `"copy_text":{"text":"maply #1"}`) {
			t.Fatalf("wire names: reply_markup = %s", raw)
		}
		buttons := sent.InlineKeyboard[0]
		if buttons[0].WebApp == nil || buttons[0].WebApp.URL != "https://maply.example/" {
			t.Fatalf("web_app = %+v", buttons[0])
		}
		if buttons[1].CopyText == nil || buttons[1].CopyText.Text != "maply #1" {
			t.Fatalf("copy_text = %+v", buttons[1])
		}
	})

	t.Run("by upload", func(t *testing.T) {
		var requests int
		handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			requests++
			handler(w, r)
		})
		_, err := c.SendPhoto(context.Background(), 42, InputFileUpload{Filename: "maply.png", Data: strings.NewReader("png")}, CaptionOptions{Markup: markup})
		if err != nil {
			t.Fatalf("SendPhoto: %v", err)
		}
		if requests != 1 || got.method != "sendPhoto" {
			t.Fatalf("requests = %d, method = %q, want one sendPhoto", requests, got.method)
		}
		var sent InlineKeyboardMarkup
		if err := json.Unmarshal([]byte(got.fields["reply_markup"]), &sent); err != nil {
			t.Fatalf("reply_markup is not JSON: %v", err)
		}
		if !strings.Contains(got.fields["reply_markup"], `"web_app":{"url":"https://maply.example/"}`) {
			t.Fatalf("wire names: reply_markup = %q", got.fields["reply_markup"])
		}
		if len(sent.InlineKeyboard) != 1 || sent.InlineKeyboard[0][0].WebApp == nil || sent.InlineKeyboard[0][0].WebApp.URL != "https://maply.example/" {
			t.Fatalf("reply_markup = %q", got.fields["reply_markup"])
		}
	})
}

const animationResult = `{"ok":true,"result":{"message_id":30,"chat":{"id":42,"type":"private"},"date":1,` +
	`"animation":{"file_id":"CgACnew","file_unique_id":"AgADu","width":720,"height":900,"duration":14,"mime_type":"video/mp4"},` +
	`"document":{"file_id":"CgACnew","file_unique_id":"AgADu","mime_type":"video/mp4"}}}`

// A file_id costs no upload, so an animation by file_id is the same JSON body
// every other send makes, with the options the issue lists.
func TestSendAnimationByFileIDStaysJSON(t *testing.T) {
	handler, method, body := capture(t, animationResult)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendAnimation(context.Background(), 42, InputFileString{Data: "CgACknown"}, AnimationOptions{
		CaptionOptions: CaptionOptions{
			ThreadID: 3, ReplyTo: 11, Caption: "<b>loop</b>",
			Markup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "again", CallbackData: "again"}}}},
		},
		Width: 720, Height: 900, Duration: 14, ShowCaptionAboveMedia: true, HasSpoiler: true,
	})
	if err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	if *method != "sendAnimation" {
		t.Fatalf("method = %q", *method)
	}
	if msg.MessageID != 30 || msg.Animation == nil || msg.Animation.FileID != "CgACnew" {
		t.Fatalf("message = %+v, want the animation's file_id back", msg)
	}
	if msg.Document == nil || msg.Document.FileID != "CgACnew" {
		t.Fatalf("document = %+v, want the backward-compatible copy", msg.Document)
	}
	want := map[string]any{
		"chat_id": float64(42), "animation": "CgACknown", "caption": "<b>loop</b>", "parse_mode": "HTML",
		"width": float64(720), "height": float64(900), "duration": float64(14),
		"show_caption_above_media": true, "has_spoiler": true, "message_thread_id": float64(3),
	}
	for key, value := range want {
		if (*body)[key] != value {
			t.Errorf("%s = %v, want %v", key, (*body)[key], value)
		}
	}
	if (*body)["reply_markup"] == nil || (*body)["reply_parameters"] == nil {
		t.Errorf("reply_markup = %v, reply_parameters = %v", (*body)["reply_markup"], (*body)["reply_parameters"])
	}
}

func TestSendAnimationOmitsWhatWasNotAskedFor(t *testing.T) {
	handler, _, body := capture(t, animationResult)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendAnimation(context.Background(), 42, InputFileString{Data: "id"}, AnimationOptions{}); err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	for _, absent := range []string{"caption", "parse_mode", "width", "height", "duration", "show_caption_above_media", "has_spoiler", "reply_markup", "message_thread_id"} {
		if _, has := (*body)[absent]; has {
			t.Fatalf("%s must be absent when it was not asked for", absent)
		}
	}
}

// maply uploaded an MP4 as a document with its own HTTP client to get an
// animation. The upload is now a multipart sendAnimation like any other.
func TestSendAnimationUploadsBytesAsMultipart(t *testing.T) {
	handler, got := captureUpload(t, animationResult)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendAnimation(context.Background(), 42, InputFileUpload{Filename: "loop.mp4", Data: strings.NewReader("mp4bytes")}, AnimationOptions{
		CaptionOptions: CaptionOptions{Caption: "preview", Markup: &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{
			{Text: "Open", WebApp: &WebAppInfo{URL: "https://framy.example/app"}},
		}}}},
		Width: 720, Height: 900, Duration: 14, HasSpoiler: true,
	})
	if err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	if got.method != "sendAnimation" {
		t.Fatalf("method = %q", got.method)
	}
	if got.files["animation"] != "mp4bytes" || got.names["animation"] != "loop.mp4" {
		t.Fatalf("animation part = %q named %q", got.files["animation"], got.names["animation"])
	}
	if _, has := got.fields["animation"]; has {
		t.Fatal("an uploaded file must not also travel as a string parameter")
	}
	if got.fields["width"] != "720" || got.fields["height"] != "900" || got.fields["duration"] != "14" || got.fields["has_spoiler"] != "true" {
		t.Fatalf("scalar fields = %v", got.fields)
	}
	if !strings.Contains(got.fields["reply_markup"], `"web_app":{"url":"https://framy.example/app"}`) {
		t.Fatalf("reply_markup = %q", got.fields["reply_markup"])
	}
	if msg.Animation == nil || msg.Animation.FileID != "CgACnew" {
		t.Fatalf("message = %+v", msg)
	}
}

// On a self-hosted server a path on its disk is sent by reference, so an
// animation larger than an upload allows costs the process no memory.
func TestSendAnimationByLocalPath(t *testing.T) {
	handler, method, body := capture(t, animationResult)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendAnimation(context.Background(), 42, InputFileLocal{Path: "/data/results/loop.mp4"}, AnimationOptions{}); err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	if *method != "sendAnimation" || (*body)["animation"] != "file:///data/results/loop.mp4" {
		t.Fatalf("method = %q, animation = %v", *method, (*body)["animation"])
	}

	cloud := New(testToken)
	if _, err := cloud.SendAnimation(context.Background(), 42, InputFileLocal{Path: "/data/results/loop.mp4"}, AnimationOptions{}); err == nil ||
		!strings.Contains(err.Error(), "self-hosted") {
		t.Fatalf("err = %v, want a refusal naming the self-hosted server", err)
	}
}

func TestSendAnimationRejectsAMissingFileBeforeSending(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a missing file must not reach the network")
	})
	if _, err := c.SendAnimation(context.Background(), 1, nil, AnimationOptions{}); err == nil {
		t.Fatal("want a refusal for a missing file")
	}
	if _, err := c.SendAnimation(context.Background(), 1, InputFileString{}, AnimationOptions{}); err == nil {
		t.Fatal("want a refusal for an empty identifier")
	}
}

// framy's preview is a photo that turns into the generating loop and then into
// the result, all through editMessageMedia.
func TestEditMessageMediaTurnsAPhotoIntoAnAnimation(t *testing.T) {
	t.Run("upload", func(t *testing.T) {
		handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":7}}`)
		c, _ := newTestClient(t, handler)

		err := c.EditMessageMedia(context.Background(), 42, 7, InputMediaAnimation{
			Media:   InputFileUpload{Filename: "loop.mp4", Data: strings.NewReader("mp4")},
			Caption: "<i>generating</i>", Width: 720, Height: 900, Duration: 14,
			ShowCaptionAboveMedia: true, HasSpoiler: true,
		}, nil)
		if err != nil {
			t.Fatalf("EditMessageMedia: %v", err)
		}
		var media map[string]any
		if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
			t.Fatalf("media is not JSON: %v", err)
		}
		want := map[string]any{
			"type": "animation", "media": "attach://media", "caption": "<i>generating</i>", "parse_mode": "HTML",
			"width": float64(720), "height": float64(900), "duration": float64(14),
			"show_caption_above_media": true, "has_spoiler": true,
		}
		for key, value := range want {
			if media[key] != value {
				t.Errorf("media.%s = %v, want %v", key, media[key], value)
			}
		}
		if got.files["media"] != "mp4" {
			t.Fatalf("uploaded part = %q", got.files["media"])
		}
	})

	t.Run("file_id", func(t *testing.T) {
		handler, method, body := capture(t, `{"ok":true,"result":{"message_id":7}}`)
		c, _ := newTestClient(t, handler)

		err := c.EditMessageMedia(context.Background(), 42, 7, InputMediaAnimation{Media: InputFileString{Data: "CgACknown"}}, nil)
		if err != nil {
			t.Fatalf("EditMessageMedia: %v", err)
		}
		media, _ := (*body)["media"].(map[string]any)
		if *method != "editMessageMedia" || media["type"] != "animation" || media["media"] != "CgACknown" {
			t.Fatalf("method = %q, media = %v", *method, media)
		}
		for _, absent := range []string{"caption", "parse_mode", "width", "height", "duration", "show_caption_above_media", "has_spoiler"} {
			if _, has := media[absent]; has {
				t.Errorf("media.%s must be absent when it was not asked for", absent)
			}
		}
	})
}

// Telegram takes photos, videos, audio and documents in an album. An animation
// would be refused after the other items were already uploaded.
func TestSendMediaGroupRefusesAnAnimation(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an album with an animation must not reach the network")
	})
	for name, item := range map[string]InputMedia{
		"value":   InputMediaAnimation{Media: InputFileString{Data: "id"}},
		"pointer": &InputMediaAnimation{Media: InputFileString{Data: "id"}},
	} {
		_, err := c.SendMediaGroup(context.Background(), 1, []InputMedia{InputMediaPhoto{Media: InputFileString{Data: "p"}}, item}, 0)
		if err == nil || !strings.Contains(err.Error(), "media[1]") {
			t.Fatalf("%s: err = %v, want a refusal naming the item", name, err)
		}
	}
}

// framy keeps the file_id of every result for inline mode and for sending it
// again, and an edit that uploads the result is the only place it appears.
func TestEditMessageMediaMessageReturnsTheNewFileID(t *testing.T) {
	var requests int
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":7,"chat":{"id":42,"type":"private"},"date":1,`+
		`"photo":[{"file_id":"AgACsmall","file_unique_id":"AQADs","width":90,"height":60},{"file_id":"AgACnew","file_unique_id":"AQADn","width":1280,"height":853}],`+
		`"caption":"<b>done</b>","show_caption_above_media":true,"has_media_spoiler":true}}`)
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		requests++
		handler(w, r)
	})

	msg, err := c.EditMessageMediaMessage(context.Background(), 42, 7,
		InputMediaPhoto{
			Media: InputFileUpload{Filename: "result.jpg", Data: strings.NewReader("jpeg")}, Caption: "<b>done</b>",
			ShowCaptionAboveMedia: true, HasSpoiler: true,
		},
		&InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "again", CallbackData: "again"}}}})
	if err != nil {
		t.Fatalf("EditMessageMediaMessage: %v", err)
	}
	if requests != 1 || got.method != "editMessageMedia" {
		t.Fatalf("requests = %d, method = %q: the caption must not cost a second call", requests, got.method)
	}
	if len(msg.Photo) != 2 || msg.Photo[1].FileID != "AgACnew" {
		t.Fatalf("photo = %+v, want the new file_id back", msg.Photo)
	}
	if msg.MessageID != 7 || !msg.HasMediaSpoiler {
		t.Fatalf("message = %+v", msg)
	}
	var media map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	want := map[string]any{
		"type": "photo", "media": "attach://media", "caption": "<b>done</b>", "parse_mode": "HTML",
		"show_caption_above_media": true, "has_spoiler": true,
	}
	for key, value := range want {
		if media[key] != value {
			t.Errorf("media.%s = %v, want %v", key, media[key], value)
		}
	}
	if got.files["media"] != "jpeg" || got.fields["reply_markup"] == "" {
		t.Fatalf("uploaded part = %q, reply_markup = %q", got.files["media"], got.fields["reply_markup"])
	}
}

func TestEditMessageMediaMessageReturnsAnAnimationsFileID(t *testing.T) {
	handler, _, body := capture(t, animationResult)
	c, _ := newTestClient(t, handler)

	msg, err := c.EditMessageMediaMessage(context.Background(), 42, 7,
		InputMediaVideo{Media: InputFileString{Data: "BAACknown"}, Caption: "clip", ShowCaptionAboveMedia: true, HasSpoiler: true}, nil)
	if err != nil {
		t.Fatalf("EditMessageMediaMessage: %v", err)
	}
	if msg.Animation == nil || msg.Animation.FileID != "CgACnew" {
		t.Fatalf("message = %+v", msg)
	}
	media, _ := (*body)["media"].(map[string]any)
	if media["type"] != "video" || media["show_caption_above_media"] != true || media["has_spoiler"] != true || media["parse_mode"] != "HTML" {
		t.Fatalf("media = %v", media)
	}
	if _, has := (*body)["reply_markup"]; has {
		t.Fatal("reply_markup must be absent when no keyboard was given")
	}
}

// The placement flags are part of every item of an album too.
func TestAlbumItemsCarryTheirOwnSpoilerAndCaptionPlacement(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaPhoto{Media: InputFileString{Data: "p"}, Caption: "first", HasSpoiler: true},
		InputMediaVideo{Media: InputFileUpload{Filename: "v.mp4", Data: strings.NewReader("mp4")}, ShowCaptionAboveMedia: true},
	}, 0)
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &items); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if items[0]["has_spoiler"] != true || items[0]["show_caption_above_media"] != nil {
		t.Fatalf("first item = %v, want a spoiler and no placement", items[0])
	}
	if items[1]["show_caption_above_media"] != true || items[1]["has_spoiler"] != nil {
		t.Fatalf("second item = %v, want a placement and no spoiler", items[1])
	}
}

// The one-value method swallows "message is not modified". The method that
// returns the message cannot: it has nothing to return, and a zero Message with
// a nil error would look like an edit that worked.
func TestEditMessageMediaMessageReportsNotModifiedAsTheError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`))
	})

	msg, err := c.EditMessageMediaMessage(context.Background(), 1, 2, InputMediaPhoto{Media: InputFileString{Data: "id"}}, nil)
	if !IsMessageNotModified(err) {
		t.Fatalf("err = %v, want the not-modified error", err)
	}
	if msg.MessageID != 0 {
		t.Fatalf("message = %+v, want none", msg)
	}
	if err := c.EditMessageMedia(context.Background(), 1, 2, InputMediaPhoto{Media: InputFileString{Data: "id"}}, nil); err != nil {
		t.Fatalf("EditMessageMedia must keep treating it as success: %v", err)
	}
}

func TestEditMessageMediaMessageRefusesMissingMediaBeforeSending(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an edit without media must not reach the network")
	})
	if _, err := c.EditMessageMediaMessage(context.Background(), 1, 2, nil, nil); err == nil {
		t.Fatal("want a refusal for nil media")
	}
	if _, err := c.EditMessageMediaMessage(context.Background(), 1, 2, InputMediaPhoto{}, nil); err == nil {
		t.Fatal("want a refusal for media with no file")
	}
}

func TestEditMessageCaptionSendsHTMLPlacementAndKeyboard(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":7,"chat":{"id":42,"type":"private"},"date":1,`+
		`"photo":[{"file_id":"AgACp","file_unique_id":"AQADp","width":10,"height":10}],"caption":"<b>$0.04</b> · 0:12","caption_entities":[{"type":"bold","offset":0,"length":5}]}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.EditMessageCaption(context.Background(), 42, 7, "<b>$0.04</b> · 0:12", EditCaptionOptions{
		Markup:                &InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{{{Text: "Stop", CallbackData: "stop"}}}},
		ShowCaptionAboveMedia: true,
	})
	if err != nil {
		t.Fatalf("EditMessageCaption: %v", err)
	}
	if *method != "editMessageCaption" {
		t.Fatalf("method = %q", *method)
	}
	want := map[string]any{
		"chat_id": float64(42), "message_id": float64(7), "caption": "<b>$0.04</b> · 0:12",
		"parse_mode": "HTML", "show_caption_above_media": true,
	}
	for key, value := range want {
		if (*body)[key] != value {
			t.Errorf("%s = %v, want %v", key, (*body)[key], value)
		}
	}
	raw, _ := json.Marshal((*body)["reply_markup"])
	if !strings.Contains(string(raw), `"callback_data":"stop"`) {
		t.Fatalf("reply_markup = %s", raw)
	}
	if msg.MessageID != 7 || msg.Caption != "<b>$0.04</b> · 0:12" || len(msg.Photo) != 1 || len(msg.CaptionEntities) != 1 {
		t.Fatalf("message = %+v, want the edited message back", msg)
	}
}

func TestEditMessageCaptionWithoutKeyboardOrPlacement(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":7}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.EditMessageCaption(context.Background(), 42, 7, "plain", EditCaptionOptions{}); err != nil {
		t.Fatalf("EditMessageCaption: %v", err)
	}
	if (*body)["caption"] != "plain" || (*body)["parse_mode"] != "HTML" {
		t.Fatalf("body = %v", *body)
	}
	for _, absent := range []string{"reply_markup", "show_caption_above_media"} {
		if _, has := (*body)[absent]; has {
			t.Fatalf("%s must be absent when it was not asked for", absent)
		}
	}
}

// An empty caption is how a caption is removed, so it has to be sent.
func TestEditMessageCaptionSendsAnEmptyCaptionToRemoveIt(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":7}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.EditMessageCaption(context.Background(), 42, 7, "", EditCaptionOptions{}); err != nil {
		t.Fatalf("EditMessageCaption: %v", err)
	}
	caption, has := (*body)["caption"]
	if !has || caption != "" {
		t.Fatalf("caption = %v (present %v), want an empty string on the wire", caption, has)
	}
	if _, has := (*body)["parse_mode"]; has {
		t.Fatal("parse_mode has nothing to parse without a caption")
	}
}

func TestEditMessageCaptionReportsNotModifiedAsTheError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified: specified new message content and reply markup are exactly the same as a current content and reply markup of the message"}`))
	})
	msg, err := c.EditMessageCaption(context.Background(), 1, 2, "same", EditCaptionOptions{})
	if !IsMessageNotModified(err) || msg.MessageID != 0 {
		t.Fatalf("message = %+v, err = %v, want the not-modified error and no message", msg, err)
	}
}
