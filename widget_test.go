// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// A widget goes out with its code, its state and the usual message options,
// and what is left at zero is not sent at all.
func TestSendWidgetSendsTheCodeAndState(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":5,"chat":{"id":42,"type":"private"},"date":1,
		"widget":{"id":"w-1","height":120,"html":"<b>hi</b>","public_data":"{\"n\":1}"}}}`)
	c, _ := newTestClient(t, handler)

	msg, err := c.SendWidget(context.Background(), 42, WidgetOptions{
		HTML: "<b>hi</b>", Height: 120, PublicData: `{"n":1}`, AllowQueries: true, IsShared: true,
		Caption: "<i>today</i>", ReplyTo: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Widget == nil || msg.Widget.ID != "w-1" || msg.Widget.Height != 120 || msg.Widget.PublicData != `{"n":1}` {
		t.Fatalf("widget = %+v", msg.Widget)
	}
	want := map[string]any{
		"chat_id": float64(42), "html": "<b>hi</b>", "height": float64(120), "public_data": `{"n":1}`,
		"allow_queries": true, "is_shared": true, "caption": "<i>today</i>", "parse_mode": "HTML",
		"reply_parameters": map[string]any{"message_id": float64(3), "allow_sending_without_reply": true},
	}
	if *method != "sendWidget" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

func TestSendSharedWidgetNamesTheWidget(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":{"message_id":6}}`)
	c, _ := newTestClient(t, handler)

	if _, err := c.SendSharedWidget(context.Background(), 42, "w-1", SharedWidgetOptions{ChatData: "x"}); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"chat_id": float64(42), "widget_id": "w-1", "chat_data": "x"}
	if *method != "sendSharedWidget" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

// What Telegram would refuse on its limits is refused here, before a request.
func TestAWidgetTelegramWouldRefuseIsNotSent(t *testing.T) {
	sent := false
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		sent = true
		_, _ = w.Write([]byte(`{"ok":true,"result":true}`))
	})
	ctx := context.Background()
	long := strings.Repeat("a", MaxWidgetDataBytes+1)
	empty := ""
	for name, err := range map[string]error{
		"no html":           second(c.SendWidget(ctx, 1, WidgetOptions{})),
		"html too long":     second(c.SendWidget(ctx, 1, WidgetOptions{HTML: strings.Repeat("a", MaxWidgetHTMLBytes+1)})),
		"chat data":         second(c.SendWidget(ctx, 1, WidgetOptions{HTML: "x", ChatData: long})),
		"no widget id":      second(c.SendSharedWidget(ctx, 1, "", SharedWidgetOptions{})),
		"edit nothing":      c.EditMessageWidgetData(ctx, 1, 2, WidgetData{}),
		"edit too long":     c.EditMessageWidgetData(ctx, 1, 2, WidgetData{PublicData: &long}),
		"no inline id":      c.EditInlineMessageWidgetData(ctx, "", WidgetData{ChatData: &empty}),
		"cache time":        c.EditSharedWidgetData(ctx, "w-1", "", 4),
		"no query id":       c.AnswerWidgetQuery(ctx, "", ""),
		"answer too long":   c.AnswerWidgetQuery(ctx, "q", long),
		"shared no id":      c.EditSharedWidgetData(ctx, "", "", 0),
		"shared too long":   c.EditSharedWidgetData(ctx, "w-1", long, 0),
		"inline edit empty": c.EditInlineMessageWidgetData(ctx, "i", WidgetData{}),
	} {
		if err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if sent {
		t.Fatal("a refused call reached Telegram")
	}
}

func second(_ Message, err error) error { return err }

// nil leaves a value alone, a pointer to "" clears it: both are sent as
// Telegram reads them, and only what changes is in the request.
func TestEditMessageWidgetDataSendsOnlyWhatChanges(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	cleared := ""
	if err := c.EditMessageWidgetData(context.Background(), 42, 7, WidgetData{ChatData: &cleared}); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"chat_id": float64(42), "message_id": float64(7), "chat_data": ""}
	if *method != "editMessageWidgetData" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}

	state := `{"rev":2}`
	*body = nil
	if err := c.EditInlineMessageWidgetData(context.Background(), "AAAB", WidgetData{PublicData: &state}); err != nil {
		t.Fatal(err)
	}
	want = map[string]any{"inline_message_id": "AAAB", "public_data": state}
	if *method != "editMessageWidgetData" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

func TestEditSharedWidgetDataAndAnswer(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)
	ctx := context.Background()

	if err := c.EditSharedWidgetData(ctx, "w-1", `{"n":2}`, 0); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"widget_id": "w-1", "public_data": `{"n":2}`}; *method != "editSharedWidgetData" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
	*body = nil
	if err := c.EditSharedWidgetData(ctx, "w-1", "", 60); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"widget_id": "w-1", "public_data": "", "cache_time": float64(60)}; !reflect.DeepEqual(*body, want) {
		t.Fatalf("%v", *body)
	}
	*body = nil
	if err := c.AnswerWidgetQuery(ctx, "q-1", "ok"); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"widget_query_id": "q-1", "data": "ok"}; *method != "answerWidgetQuery" || !reflect.DeepEqual(*body, want) {
		t.Fatalf("%s %v", *method, *body)
	}
}

// The two query updates decode with their kind: plain data with a wallet
// card, and a transaction the wallet reports as sent.
func TestWidgetQueriesDecode(t *testing.T) {
	raw := `[{"update_id":1,"widget_query":{"id":"q-1","from":{"id":7,"is_bot":false,"first_name":"A"},
		"chat":{"id":7,"type":"private"},"message_id":5,"chat_instance":"ci","data":"vote:2","widget_id":"w-1",
		"type":{"type":"data","wallet_card":{"address":"UQ…","amount":"1500000000","stickers":[]}}}},
	{"update_id":2,"shared_widget_query":{"id":"q-2","from":{"id":8,"is_bot":false,"first_name":"B"},"data":"pay","widget_id":"w-1",
		"type":{"type":"sent_transaction","wallet":{"address":"UQ…","amount":"1"},"msg_hash":"abc"}}}]`
	var updates []Update
	if err := json.Unmarshal([]byte(raw), &updates); err != nil {
		t.Fatal(err)
	}
	q := updates[0].WidgetQuery
	if q == nil || q.ID != "q-1" || q.Chat == nil || q.MessageID != 5 || q.Data != "vote:2" || q.WidgetID != "w-1" ||
		q.Type.Type != WidgetQueryData || q.Type.WalletCard == nil || q.Type.WalletCard.Amount != "1500000000" {
		t.Fatalf("widget_query = %+v", q)
	}
	s := updates[1].SharedWidgetQuery
	if s == nil || s.From.ID != 8 || s.Type.Type != WidgetQuerySentTransaction || s.Type.MsgHash != "abc" || s.Type.Wallet == nil {
		t.Fatalf("shared_widget_query = %+v", s)
	}
}

func TestWidgetInlineContentMarshalsWithItsCaptionMode(t *testing.T) {
	for _, c := range []struct {
		content InputMessageContent
		want    string
	}{
		{InputWidgetMessageContent{HTML: "<b>x</b>", AllowQueries: true}, `{"html":"<b>x</b>","allow_queries":true}`},
		{InputSharedWidgetMessageContent{WidgetID: "w-1", Caption: "<i>c</i>"}, `{"parse_mode":"HTML","widget_id":"w-1","caption":"<i>c</i>"}`},
	} {
		got, err := json.Marshal(c.content)
		if err != nil {
			t.Fatal(err)
		}
		var gotMap, wantMap map[string]any
		_ = json.Unmarshal(got, &gotMap)
		_ = json.Unmarshal([]byte(c.want), &wantMap)
		if !reflect.DeepEqual(gotMap, wantMap) {
			t.Errorf("%T = %s, want %s", c.content, got, c.want)
		}
	}
}

// The estimate is zlib at level 9: repetitive HTML shrinks far below its size.
func TestWidgetCompressedSize(t *testing.T) {
	html := strings.Repeat("<div class=cell></div>", 1000)
	if n := WidgetCompressedSize(html); n <= 0 || n >= MaxWidgetCompressedHTMLBytes {
		t.Fatalf("compressed = %d", n)
	}
}

// The widget methods can be required at startup: each is probed with an empty
// body, and a server older than 10.4 is named as lacking it.
func TestPreflightCanRequireTheWidgetMethods(t *testing.T) {
	methods := []string{"answerWidgetQuery", "editMessageWidgetData", "sendSharedWidget", "sendWidget"}
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/getMe") {
			_, _ = w.Write([]byte(`{"ok":true,"result":{"id":7,"username":"maplyin_bot"}}`))
			return
		}
		oldServer(map[string]bool{})(w, r)
	})
	_, err := c.Preflight(context.Background(), Needs{Methods: methods})
	var missing *MissingMethodsError
	if !errors.As(err, &missing) || !reflect.DeepEqual(missing.Methods, methods) {
		t.Fatalf("err = %v, want all four missing", err)
	}
	if _, err := c.Probe(context.Background(), "editSharedWidgetData"); err == nil {
		t.Fatal("editSharedWidgetData reaches TDLib on an empty body and must not be probed")
	}
}
