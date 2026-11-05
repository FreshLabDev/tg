// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// A thumbnail is always a new upload, so a video named by a path on a --local
// server still makes the request multipart: the path stays a field, the
// thumbnail is a part of its own.
func TestSendVideoUploadsItsThumbnailNextToALocalPath(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":3}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendVideo(context.Background(), 42, InputFileLocal{Path: "/var/lib/telegram-bot-api/clip.mp4"}, VideoOptions{
		Width: 1080, Height: 1920, SupportsStreaming: true,
		Thumbnail: &InputFileUpload{Filename: "thumb.jpg", Data: strings.NewReader("jpeg")},
	})
	if err != nil {
		t.Fatalf("SendVideo: %v", err)
	}
	if got.fields["video"] != "file:///var/lib/telegram-bot-api/clip.mp4" {
		t.Fatalf("video = %q, want the path as a field", got.fields["video"])
	}
	if got.fields["thumbnail"] != "attach://thumbnail" || got.files["thumbnail"] != "jpeg" || got.names["thumbnail"] != "thumb.jpg" {
		t.Fatalf("thumbnail field %q, parts %v", got.fields["thumbnail"], got.files)
	}
	if len(got.files) != 1 {
		t.Fatalf("parts = %v, want only the thumbnail uploaded", got.files)
	}
}

func TestSendAudioAndDocumentCarryAThumbnail(t *testing.T) {
	thumb := func() *InputFileUpload {
		return &InputFileUpload{Filename: "cover.jpg", Data: strings.NewReader("jpeg")}
	}
	for name, send := range map[string]func(*Client) error{
		"sendAudio": func(c *Client) error {
			_, err := c.SendAudio(context.Background(), 42, InputFileString{Data: "audio-id"}, AudioOptions{Title: "Song", Thumbnail: thumb()})
			return err
		},
		"sendDocument": func(c *Client) error {
			_, err := c.SendDocument(context.Background(), 42, InputFileString{Data: "doc-id"}, DocumentOptions{DisableContentTypeDetection: true, Thumbnail: thumb()})
			return err
		},
	} {
		handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":3}}`)
		c, _ := newTestClient(t, handler)
		if err := send(c); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got.method != name || got.fields["thumbnail"] != "attach://thumbnail" || got.files["thumbnail"] != "jpeg" {
			t.Fatalf("%s: method %q, fields %v, parts %v", name, got.method, got.fields, got.files)
		}
	}
}

// Without a thumbnail nothing changes: a file_id stays JSON, with no
// thumbnail field for Telegram to misread.
func TestNoThumbnailLeavesTheRequestAlone(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":3}}`)
	c, _ := newTestClient(t, handler)
	if _, err := c.SendDocument(context.Background(), 42, InputFileString{Data: "doc-id"}, DocumentOptions{}); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	if _, has := (*body)["thumbnail"]; has || *method != "sendDocument" {
		t.Fatalf("method %q, body %v", *method, *body)
	}
}

// Each item of an album carries its own thumbnail as its own part. None is
// named plain "thumbnail": the server gives a part of that name to every item
// that has no thumbnail field of its own.
func TestAlbumItemsCarryTheirOwnThumbnails(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2},{"message_id":3}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaVideo{Media: InputFileLocal{Path: "/data/a.mp4"}, SupportsStreaming: true,
			Thumbnail: &InputFileUpload{Filename: "a.jpg", Data: strings.NewReader("jpeg-a")}},
		InputMediaPhoto{Media: InputFileString{Data: "photo-id"}},
		InputMediaVideo{Media: InputFileString{Data: "video-id"},
			Thumbnail: &InputFileUpload{Filename: "c.jpg", Data: strings.NewReader("jpeg-c")}},
	}, MediaGroupOptions{})
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &items); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if items[0]["media"] != "file:///data/a.mp4" || items[0]["thumbnail"] != "attach://thumbnail0" {
		t.Fatalf("first item = %v", items[0])
	}
	if _, has := items[1]["thumbnail"]; has {
		t.Fatalf("second item = %v, want no thumbnail", items[1])
	}
	if items[2]["media"] != "video-id" || items[2]["thumbnail"] != "attach://thumbnail2" {
		t.Fatalf("third item = %v", items[2])
	}
	want := map[string]string{"thumbnail0": "jpeg-a", "thumbnail2": "jpeg-c"}
	if !reflect.DeepEqual(got.files, want) {
		t.Fatalf("parts = %v, want %v", got.files, want)
	}
}

func TestAnAlbumOfDocumentsCarriesThumbnails(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaDocument{Media: InputFileUpload{Filename: "a.mp4", Data: strings.NewReader("a")}, DisableContentTypeDetection: true,
			Thumbnail: &InputFileUpload{Filename: "a.jpg", Data: strings.NewReader("jpeg-a")}},
		InputMediaDocument{Media: InputFileString{Data: "doc-id"}, Caption: "two"},
	}, MediaGroupOptions{})
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &items); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if items[0]["type"] != "document" || items[0]["media"] != "attach://media0" || items[0]["thumbnail"] != "attach://thumbnail0" ||
		items[0]["disable_content_type_detection"] != true {
		t.Fatalf("first item = %v", items[0])
	}
	if got.files["media0"] != "a" || got.files["thumbnail0"] != "jpeg-a" {
		t.Fatalf("parts = %v", got.files)
	}
}

func TestAnAlbumOfAudio(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaAudio{Media: InputFileString{Data: "a1"}, Duration: 181, Performer: "Artist", Title: "One"},
		InputMediaAudio{Media: InputFileString{Data: "a2"}, Caption: "<b>side B</b>"},
	}, MediaGroupOptions{})
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	want := []any{
		map[string]any{"type": "audio", "media": "a1", "duration": float64(181), "performer": "Artist", "title": "One"},
		map[string]any{"type": "audio", "media": "a2", "caption": "<b>side B</b>", "parse_mode": "HTML"},
	}
	if *method != "sendMediaGroup" || !reflect.DeepEqual((*body)["media"], want) {
		t.Fatalf("method %q, media = %v", *method, (*body)["media"])
	}
}

// Telegram groups documents only with documents and audio only with audio,
// and that is checked before anything is sent.
func TestAnAlbumMixingDocumentsOrAudioIsRefused(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a mixed album must not reach the network")
	})
	photo := InputMediaPhoto{Media: InputFileString{Data: "p"}}
	doc := InputMediaDocument{Media: InputFileString{Data: "d"}}
	audio := InputMediaAudio{Media: InputFileString{Data: "a"}}
	for name, album := range map[string][]InputMedia{
		"photo and document": {photo, doc},
		"audio and photo":    {audio, photo},
		"audio and document": {audio, doc},
	} {
		if _, err := c.SendMediaGroup(context.Background(), 1, album, MediaGroupOptions{}); err == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
	if _, err := c.SendMediaGroup(context.Background(), 1, []InputMedia{photo, nil}, MediaGroupOptions{}); err == nil {
		t.Fatal("a nil item: want a refusal")
	}
}

// An album answers a message like every other reply here, so a deleted
// original does not fail it, and it goes silently and protected when asked.
func TestSendMediaGroupRepliesQuietly(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaPhoto{Media: InputFileString{Data: "p1"}},
		InputMediaPhoto{Media: InputFileString{Data: "p2"}},
	}, MediaGroupOptions{ThreadID: 9, ReplyTo: 77, DisableNotification: true, ProtectContent: true, MessageEffectID: "fx"})
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	want := map[string]any{
		"chat_id":              float64(42),
		"message_thread_id":    float64(9),
		"reply_parameters":     map[string]any{"message_id": float64(77), "allow_sending_without_reply": true},
		"disable_notification": true,
		"protect_content":      true,
		"message_effect_id":    "fx",
		"media": []any{
			map[string]any{"type": "photo", "media": "p1"},
			map[string]any{"type": "photo", "media": "p2"},
		},
	}
	if !reflect.DeepEqual(*body, want) {
		t.Fatalf("body =\n%v\nwant\n%v", *body, want)
	}
}

// A live photo is two files in one send; two uploads are two parts.
func TestSendLivePhotoUploadsTheVideoAndTheStill(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":5,"date":1,"chat":{"id":42,"type":"private"},
		"photo":[{"file_id":"still","file_unique_id":"s","width":1080,"height":1440}],
		"live_photo":{"file_id":"motion","file_unique_id":"m","width":1080,"height":1440,"duration":3,
			"photo":[{"file_id":"still","file_unique_id":"s","width":1080,"height":1440}]}}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendLivePhoto(context.Background(), 42,
		InputFileUpload{Filename: "live.mov", Data: strings.NewReader("mov")},
		InputFileUpload{Filename: "still.jpg", Data: strings.NewReader("jpeg")},
		LivePhotoOptions{CaptionOptions: CaptionOptions{Caption: "<i>live</i>", ReplyTo: 11}, ShowCaptionAboveMedia: true, HasSpoiler: true})
	if err != nil {
		t.Fatalf("SendLivePhoto: %v", err)
	}
	if got.method != "sendLivePhoto" || got.files["live_photo"] != "mov" || got.files["photo"] != "jpeg" || got.names["live_photo"] != "live.mov" {
		t.Fatalf("method %q, parts %v, names %v", got.method, got.files, got.names)
	}
	if got.fields["caption"] != "<i>live</i>" || got.fields["parse_mode"] != "HTML" ||
		got.fields["show_caption_above_media"] != "true" || got.fields["has_spoiler"] != "true" {
		t.Fatalf("fields = %v", got.fields)
	}
	if !strings.Contains(got.fields["reply_parameters"], `"allow_sending_without_reply":true`) {
		t.Fatalf("reply_parameters = %q", got.fields["reply_parameters"])
	}
	if msg.LivePhoto == nil || msg.LivePhoto.FileID != "motion" || len(msg.Photo) != 1 {
		t.Fatalf("message = %+v, want the live photo and its still", msg)
	}
}

func TestSendLivePhotoByLocalPathsStaysJSON(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":5}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendLivePhoto(context.Background(), 42, InputFileLocal{Path: "/jobs/1/live.mov"}, InputFileString{Data: "still-id"}, LivePhotoOptions{}); err != nil {
		t.Fatalf("SendLivePhoto: %v", err)
	}
	want := map[string]any{"chat_id": float64(42), "live_photo": "file:///jobs/1/live.mov", "photo": "still-id"}
	if *method != "sendLivePhoto" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("method %q, body %v", *method, *body)
	}
}

func TestSendLivePhotoRequiresBothFiles(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a live photo without its still must not reach the network")
	})
	if _, err := c.SendLivePhoto(context.Background(), 42, InputFileString{Data: "motion"}, nil, LivePhotoOptions{}); err == nil {
		t.Fatal("want a refusal")
	}
	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaLivePhoto{Media: InputFileString{Data: "motion"}},
		InputMediaPhoto{Media: InputFileString{Data: "p"}},
	}, MediaGroupOptions{})
	if err == nil || !strings.Contains(err.Error(), "media[0]: photo") {
		t.Fatalf("err = %v, want a refusal naming the missing still", err)
	}
}

// In an album a live photo is "live_photo" with its video in media and its
// still in photo, each file going its own way.
func TestAlbumCarriesALivePhoto(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":[{"message_id":1},{"message_id":2}]}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendMediaGroup(context.Background(), 42, []InputMedia{
		InputMediaLivePhoto{
			Media:   InputFileUpload{Filename: "live.mov", Data: strings.NewReader("mov")},
			Photo:   InputFileUpload{Filename: "still.jpg", Data: strings.NewReader("jpeg")},
			Caption: "first", HasSpoiler: true,
		},
		InputMediaLivePhoto{Media: InputFileString{Data: "motion-id"}, Photo: InputFileLocal{Path: "/jobs/1/still.jpg"}},
	}, MediaGroupOptions{ReplyTo: 3})
	if err != nil {
		t.Fatalf("SendMediaGroup: %v", err)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &items); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	first := map[string]any{"type": "live_photo", "media": "attach://media0", "photo": "attach://photo0", "caption": "first", "parse_mode": "HTML", "has_spoiler": true}
	second := map[string]any{"type": "live_photo", "media": "motion-id", "photo": "file:///jobs/1/still.jpg"}
	if !reflect.DeepEqual(items[0], first) || !reflect.DeepEqual(items[1], second) {
		t.Fatalf("items = %v", items)
	}
	if !reflect.DeepEqual(got.files, map[string]string{"media0": "mov", "photo0": "jpeg"}) {
		t.Fatalf("parts = %v", got.files)
	}
}

// An inline card becomes the audio Telegram already has, by its file_id.
func TestEditInlineMessageMediaTurnsACardIntoAudio(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.EditInlineMessageMedia(context.Background(), "im-1",
		InputMediaAudio{Media: InputFileString{Data: "audio-id"}, Caption: "<b>track</b>", Performer: "Artist", Title: "Song", Duration: 200}, nil)
	if err != nil {
		t.Fatalf("EditInlineMessageMedia: %v", err)
	}
	want := map[string]any{
		"inline_message_id": "im-1",
		"media": map[string]any{"type": "audio", "media": "audio-id", "caption": "<b>track</b>", "parse_mode": "HTML",
			"performer": "Artist", "title": "Song", "duration": float64(200)},
	}
	if *method != "editMessageMedia" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("method %q, body %v", *method, *body)
	}
}

// An inline edit takes no upload, a thumbnail included, while a video's cover
// by file_id goes with the video rather than being dropped.
func TestEditInlineMessageMediaCarriesACoverAndRefusesAThumbnail(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.EditInlineMessageMedia(context.Background(), "im-1", InputMediaVideo{Media: InputFileString{Data: "v"}, Cover: InputFileString{Data: "cover-id"}}, nil); err != nil {
		t.Fatalf("EditInlineMessageMedia: %v", err)
	}
	if media, _ := (*body)["media"].(map[string]any); media["cover"] != "cover-id" {
		t.Fatalf("media = %v, want the cover", media)
	}

	*body = nil
	err := c.EditInlineMessageMedia(context.Background(), "im-1", InputMediaVideo{Media: InputFileString{Data: "v"},
		Thumbnail: &InputFileUpload{Filename: "t.jpg", Data: strings.NewReader("jpeg")}}, nil)
	if err == nil || *body != nil {
		t.Fatalf("err = %v, body %v: want a refusal before sending", err, *body)
	}
}

// An ephemeral message's media edit carries the second files of an item too.
func TestEditEphemeralMessageMediaCarriesACover(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.EditEphemeralMessageMedia(context.Background(), -100, 7, 55,
		InputMediaVideo{Media: InputFileString{Data: "v"}, Cover: InputFileUpload{Filename: "c.jpg", Data: strings.NewReader("jpeg")}}, nil)
	if err != nil {
		t.Fatalf("EditEphemeralMessageMedia: %v", err)
	}
	var media map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if media["cover"] != "attach://cover" || got.files["cover"] != "jpeg" {
		t.Fatalf("media = %v, parts %v", media, got.files)
	}
}

// A rich message's media item keeps its own part name, and a second file of
// the item gets one next to it.
func TestRichMediaCarriesAVideoCover(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendRich(context.Background(), 42, `<img src="tg://photo?id=clip"/>`, RichOptions{Media: []RichMedia{{
		ID:    "clip",
		Media: InputMediaVideo{Media: InputFileUpload{Filename: "v.mp4", Data: strings.NewReader("mp4")}, Cover: InputFileUpload{Filename: "c.jpg", Data: strings.NewReader("jpeg")}},
	}}})
	if err != nil {
		t.Fatalf("SendRich: %v", err)
	}
	var rich struct {
		Media []struct {
			Media map[string]any `json:"media"`
		} `json:"media"`
	}
	if err := json.Unmarshal([]byte(got.fields["rich_message"]), &rich); err != nil {
		t.Fatalf("rich_message is not JSON: %v", err)
	}
	if m := rich.Media[0].Media; m["media"] != "attach://rich0" || m["cover"] != "attach://rich0cover" {
		t.Fatalf("media = %v", m)
	}
	if got.files["rich0"] != "mp4" || got.files["rich0cover"] != "jpeg" {
		t.Fatalf("parts = %v", got.files)
	}
}

// The server decodes a file:// URI, so a path goes encoded: a "#" would end
// the path, a "%" would start an escape, and a space would not survive.
func TestALocalPathIsPercentEncoded(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	for path, want := range map[string]string{
		"/files/a b#1%.mp4":      "file:///files/a%20b%231%25.mp4",
		"/files/what?.mp4":       "file:///files/what%3F.mp4",
		"/files/видео.mp4":       "file:///files/%D0%B2%D0%B8%D0%B4%D0%B5%D0%BE.mp4",
		"/srv/cache/plain_1.mp4": "file:///srv/cache/plain_1.mp4",
	} {
		if _, err := c.SendDocument(context.Background(), 42, InputFileLocal{Path: path}, DocumentOptions{}); err != nil {
			t.Fatalf("SendDocument(%q): %v", path, err)
		}
		if (*body)["document"] != want {
			t.Fatalf("%q went as %v, want %q", path, (*body)["document"], want)
		}
	}
}
