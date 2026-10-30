// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// A cover is a second file of the send: an upload goes as its own part,
// named from the cover field by an attach:// reference.
func TestAVideoGoesOutWithACoverAndAStartTime(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":9}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendVideo(context.Background(), 42, InputFileUpload{Filename: "clip.mp4", Data: strings.NewReader("mp4")}, VideoOptions{
		Cover:                 InputFileUpload{Filename: "cover.jpg", Data: strings.NewReader("jpeg")},
		StartTimestamp:        7,
		ShowCaptionAboveMedia: true,
		CaptionOptions:        CaptionOptions{Caption: "rain"},
	})
	if err != nil {
		t.Fatalf("SendVideo: %v", err)
	}
	if got.method != "sendVideo" || got.files["video"] != "mp4" || got.files["cover"] != "jpeg" || got.names["cover"] != "cover.jpg" {
		t.Fatalf("parts = %v %v", got.files, got.names)
	}
	if got.fields["cover"] != "attach://cover" {
		t.Fatalf("cover = %q, want a reference to its part", got.fields["cover"])
	}
	if got.fields["start_timestamp"] != "7" || got.fields["show_caption_above_media"] != "true" || got.fields["caption"] != "rain" {
		t.Fatalf("fields = %v", got.fields)
	}
}

// A cover Telegram has already is named by its file_id, and a send with no
// upload stays JSON.
func TestACoverByFileIDStaysJSON(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendVideo(context.Background(), 42, InputFileString{Data: "BAAC-video"}, VideoOptions{Cover: InputFileString{Data: "AgAC-cover"}}); err != nil {
		t.Fatalf("SendVideo: %v", err)
	}
	if *method != "sendVideo" || (*body)["video"] != "BAAC-video" || (*body)["cover"] != "AgAC-cover" {
		t.Fatalf("%s %v", *method, *body)
	}
	for _, k := range []string{"start_timestamp", "show_caption_above_media"} {
		if _, has := (*body)[k]; has {
			t.Errorf("%s sent unasked", k)
		}
	}
}

func TestACoverThatIsNoFileIsRefusedBeforeSending(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an empty cover must not reach the network")
	})
	_, err := c.SendVideo(context.Background(), 1, InputFileString{Data: "BAAC-video"}, VideoOptions{Cover: InputFileString{}})
	if err == nil || !strings.Contains(err.Error(), "cover") {
		t.Fatalf("err = %v, want one that names the cover", err)
	}
}

// The send options are on every captioned send, ephemeral or not.
func TestAPhotoGoesOutSilentlyWithAnEffect(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendPhoto(context.Background(), 42, InputFileString{Data: "AgAC-photo"}, CaptionOptions{
		DisableNotification: true, ProtectContent: true, MessageEffectID: "5104841245755180586",
	})
	if err != nil {
		t.Fatalf("SendPhoto: %v", err)
	}
	if *method != "sendPhoto" || (*body)["disable_notification"] != true || (*body)["protect_content"] != true ||
		(*body)["message_effect_id"] != "5104841245755180586" {
		t.Fatalf("%s %v", *method, *body)
	}
}

func TestSendOptionsAreLeftOutUnasked(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendDocument(context.Background(), 42, InputFileString{Data: "BQAC-doc"}, DocumentOptions{}); err != nil {
		t.Fatalf("SendDocument: %v", err)
	}
	for _, k := range []string{"disable_notification", "protect_content", "message_effect_id"} {
		if _, has := (*body)[k]; has {
			t.Errorf("%s sent unasked", k)
		}
	}
}

// An upload is multipart, where a boolean travels as its text.
func TestSendOptionsTravelInAnUpload(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":1}}`)
	c, _ := newTestClient(t, handler)

	_, err := c.SendAnimation(context.Background(), 42, InputFileUpload{Filename: "loop.mp4", Data: strings.NewReader("mp4")}, AnimationOptions{
		CaptionOptions: CaptionOptions{DisableNotification: true, MessageEffectID: "5046509860389126442"},
	})
	if err != nil {
		t.Fatalf("SendAnimation: %v", err)
	}
	if got.fields["disable_notification"] != "true" || got.fields["message_effect_id"] != "5046509860389126442" {
		t.Fatalf("fields = %v", got.fields)
	}
	if _, has := got.fields["protect_content"]; has {
		t.Error("protect_content sent unasked")
	}
}
