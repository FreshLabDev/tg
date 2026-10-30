// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"net/http"
	"reflect"
	"testing"
)

// Each kind of result framy and maply share, by file_id or by URL, goes as
// savePreparedInlineMessage documents it, and the answer comes back whole.
func TestAPreparedMessageOfEachKind(t *testing.T) {
	for name, tc := range map[string]struct {
		result InlineQueryResult
		want   map[string]any
	}{
		"cached photo": {
			InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "photo-id", Caption: "<i>a fox</i>"},
			map[string]any{"type": "photo", "id": "p", "photo_file_id": "photo-id", "caption": "<i>a fox</i>", "parse_mode": "HTML"},
		},
		"cached video": {
			InlineQueryResultCachedVideo{ID: "v", VideoFileID: "video-id", Title: "Rain"},
			map[string]any{"type": "video", "id": "v", "video_file_id": "video-id", "title": "Rain"},
		},
		"cached animation": {
			InlineQueryResultCachedMpeg4Gif{ID: "g", Mpeg4FileID: "gif-id"},
			map[string]any{"type": "mpeg4_gif", "id": "g", "mpeg4_file_id": "gif-id"},
		},
		"photo by URL": {
			InlineQueryResultPhoto{ID: "p", PhotoURL: "https://files.test/a.jpg", ThumbnailURL: "https://files.test/a.t.jpg", PhotoWidth: 1024, PhotoHeight: 768},
			map[string]any{"type": "photo", "id": "p", "photo_url": "https://files.test/a.jpg", "thumbnail_url": "https://files.test/a.t.jpg",
				"photo_width": 1024.0, "photo_height": 768.0},
		},
		"video by URL": {
			InlineQueryResultVideo{ID: "v", VideoURL: "https://files.test/v.mp4", ThumbnailURL: "https://files.test/v.jpg", Title: "Rain",
				Caption: "rain", VideoWidth: 1280, VideoHeight: 720, VideoDuration: 5},
			map[string]any{"type": "video", "id": "v", "video_url": "https://files.test/v.mp4", "mime_type": "video/mp4",
				"thumbnail_url": "https://files.test/v.jpg", "title": "Rain", "caption": "rain", "parse_mode": "HTML",
				"video_width": 1280.0, "video_height": 720.0, "video_duration": 5.0},
		},
		"animation by URL": {
			InlineQueryResultMpeg4Gif{ID: "g", Mpeg4URL: "https://files.test/g.mp4", ThumbnailURL: "https://files.test/g.jpg", Mpeg4Duration: 3},
			map[string]any{"type": "mpeg4_gif", "id": "g", "mpeg4_url": "https://files.test/g.mp4", "thumbnail_url": "https://files.test/g.jpg",
				"mpeg4_duration": 3.0},
		},
	} {
		t.Run(name, func(t *testing.T) {
			handler, method, body := capture(t, `{"ok":true,"result":{"id":"prep-1","expiration_date":1793000000}}`)
			c, _ := newTestClient(t, handler)
			got, err := c.SavePreparedInlineMessage(context.Background(), 7, tc.result, PreparedChats{Users: true, Groups: true})
			if err != nil {
				t.Fatal(err)
			}
			if got != (PreparedInlineMessage{ID: "prep-1", ExpirationDate: 1793000000}) {
				t.Fatalf("answer = %+v", got)
			}
			want := map[string]any{"user_id": 7.0, "result": tc.want, "allow_user_chats": true, "allow_group_chats": true}
			if *method != "savePreparedInlineMessage" || !reflect.DeepEqual(*body, want) {
				t.Fatalf("%s\n%v\nwant\n%v", *method, *body, want)
			}
		})
	}
}

func TestAPreparedMessageForAnyChat(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":{"id":"prep-1","expiration_date":1}}`)
	c, _ := newTestClient(t, handler)
	if _, err := c.SavePreparedInlineMessage(context.Background(), 7, InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "x"}, AnyChat); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"allow_user_chats", "allow_bot_chats", "allow_group_chats", "allow_channel_chats"} {
		if (*body)[name] != true {
			t.Errorf("%s = %v", name, (*body)[name])
		}
	}
}

// What Telegram would refuse is refused before anything is sent.
func TestAPreparedMessageTelegramWouldRefuseIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":{"id":"x","expiration_date":1}}`))
	})
	photo := InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "x"}
	for name, tc := range map[string]struct {
		user   int64
		result InlineQueryResult
		chats  PreparedChats
	}{
		"no user":                 {0, photo, AnyChat},
		"no result":               {7, nil, AnyChat},
		"no chat":                 {7, photo, PreparedChats{}},
		"video without a URL":     {7, InlineQueryResultVideo{ID: "v", ThumbnailURL: "t", Title: "x"}, AnyChat},
		"video without a title":   {7, InlineQueryResultVideo{ID: "v", VideoURL: "u", ThumbnailURL: "t"}, AnyChat},
		"video without its thumb": {7, InlineQueryResultVideo{ID: "v", VideoURL: "u", Title: "x"}, AnyChat},
		"animation without a URL": {7, InlineQueryResultMpeg4Gif{ID: "g", ThumbnailURL: "t"}, AnyChat},
		"animation without thumb": {7, InlineQueryResultMpeg4Gif{ID: "g", Mpeg4URL: "u"}, AnyChat},
	} {
		if _, err := c.SavePreparedInlineMessage(context.Background(), tc.user, tc.result, tc.chats); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if sent {
		t.Fatal("a refused message was sent")
	}
}

func TestAPreparedMessageWithoutAnIdIsAnError(t *testing.T) {
	handler, _, _ := capture(t, `{"ok":true,"result":{}}`)
	c, _ := newTestClient(t, handler)
	if _, err := c.SavePreparedInlineMessage(context.Background(), 7, InlineQueryResultCachedPhoto{ID: "p", PhotoFileID: "x"}, AnyChat); err == nil {
		t.Fatal("no error")
	}
}
