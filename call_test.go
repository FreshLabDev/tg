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

// A method this package does not model still uploads: the file is the part
// named after its parameter, and the answer is the whole envelope.
func TestCallMultipartUploadsUnderTheParameterName(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":{"message_id":8}}`)
	c, _ := newTestClient(t, handler)

	var out struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	err := c.CallMultipart(context.Background(), "sendVoice",
		map[string]any{"chat_id": int64(42), "duration": 3, "reply_markup": map[string]any{"inline_keyboard": []any{}}},
		map[string]InputFile{"voice": InputFileUpload{Filename: "v.ogg", Data: strings.NewReader("ogg")}}, &out)
	if err != nil {
		t.Fatalf("CallMultipart: %v", err)
	}
	if !out.OK || out.Result.MessageID != 8 {
		t.Fatalf("out = %+v", out)
	}
	if got.method != "sendVoice" || got.files["voice"] != "ogg" || got.names["voice"] != "v.ogg" {
		t.Fatalf("method %q, parts %v", got.method, got.files)
	}
	if got.fields["chat_id"] != "42" || got.fields["duration"] != "3" || got.fields["reply_markup"] != `{"inline_keyboard":[]}` {
		t.Fatalf("fields = %v", got.fields)
	}
	if _, has := got.fields["voice"]; has {
		t.Fatal("an upload must not also travel as a field")
	}
}

// With nothing to upload the request is the JSON of every other call, and a
// file named by id or path is the parameter's value.
func TestCallMultipartStaysJSONWithoutAnUpload(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":8}}`)
	c, _ := newTestClient(t, handler)

	fields := map[string]any{"chat_id": 42}
	err := c.CallMultipart(context.Background(), "sendVideoNote", fields, map[string]InputFile{
		"video_note": InputFileLocal{Path: "/data/note one.mp4"},
		"thumbnail":  InputFileString{Data: "never-mind"},
	}, nil)
	if err != nil {
		t.Fatalf("CallMultipart: %v", err)
	}
	want := map[string]any{"chat_id": float64(42), "video_note": "file:///data/note%20one.mp4", "thumbnail": "never-mind"}
	if *method != "sendVideoNote" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("method %q, body %v", *method, *body)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %v: the caller's map was modified", fields)
	}
}

// A file inside an object is referred to as attach://<name>.
func TestCallMultipartUploadsAFileAnObjectRefersTo(t *testing.T) {
	handler, got := captureUpload(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.CallMultipart(context.Background(), "sendPaidMedia", map[string]any{
		"chat_id":    42,
		"star_count": 5,
		"media":      []map[string]any{{"type": "photo", "media": "attach://p0"}},
	}, map[string]InputFile{"p0": InputFileUpload{Filename: "p.jpg", Data: strings.NewReader("jpeg")}}, nil)
	if err != nil {
		t.Fatalf("CallMultipart: %v", err)
	}
	var media []map[string]any
	if err := json.Unmarshal([]byte(got.fields["media"]), &media); err != nil {
		t.Fatalf("media is not JSON: %v", err)
	}
	if media[0]["media"] != "attach://p0" || got.files["p0"] != "jpeg" {
		t.Fatalf("media %v, parts %v", media, got.files)
	}
}

func TestCallMultipartRefusesANameThatIsBothAFieldAndAFile(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a clash must not reach the network")
	})
	err := c.CallMultipart(context.Background(), "sendVoice", map[string]any{"voice": "id"},
		map[string]InputFile{"voice": InputFileString{Data: "other"}}, nil)
	if err == nil {
		t.Fatal("want a refusal")
	}
	if err := c.CallMultipart(context.Background(), "sendVoice", nil, map[string]InputFile{"voice": nil}, nil); err == nil {
		t.Fatal("a nil file: want a refusal")
	}
}
