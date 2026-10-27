// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"net/http"
	"testing"
)

// maply and framy both point the menu button at their Mini App. A zero chat is
// the default for every private chat.
func TestSetChatMenuButtonOpensTheMiniApp(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	err := c.SetChatMenuButton(context.Background(), 0, MenuButton{
		Type: MenuWebApp, Text: "Open maply", WebApp: &WebAppInfo{URL: "https://maply.example/"},
	})
	if err != nil {
		t.Fatalf("SetChatMenuButton: %v", err)
	}
	if *method != "setChatMenuButton" {
		t.Fatalf("method = %q", *method)
	}
	if _, has := (*body)["chat_id"]; has {
		t.Fatalf("chat_id = %v, want none: it sets the default for every private chat", (*body)["chat_id"])
	}
	menu, _ := (*body)["menu_button"].(map[string]any)
	webApp, _ := menu["web_app"].(map[string]any)
	if menu["type"] != "web_app" || menu["text"] != "Open maply" || webApp["url"] != "https://maply.example/" {
		t.Fatalf("menu_button = %v", menu)
	}
}

func TestSetChatMenuButtonForOneChat(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.SetChatMenuButton(context.Background(), 5550001, MenuButton{Type: MenuCommands}); err != nil {
		t.Fatalf("SetChatMenuButton: %v", err)
	}
	if (*body)["chat_id"] != float64(5550001) {
		t.Fatalf("chat_id = %v", (*body)["chat_id"])
	}
	menu, _ := (*body)["menu_button"].(map[string]any)
	if menu["type"] != "commands" || len(menu) != 1 {
		t.Fatalf("menu_button = %v, want only the type", menu)
	}
}

// The other kinds carry nothing but their type, and an unset type is the
// default one rather than a request Telegram cannot read.
func TestSetChatMenuButtonKinds(t *testing.T) {
	tests := []struct {
		name   string
		button MenuButton
		want   string
	}{
		{"default", MenuButton{Type: MenuDefault}, "default"},
		{"unset type is the default", MenuButton{}, "default"},
		{"commands", MenuButton{Type: MenuCommands}, "commands"},
		{"text and app are dropped from the other kinds", MenuButton{
			Type: MenuCommands, Text: "ignored", WebApp: &WebAppInfo{URL: "https://example.test"},
		}, "commands"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, _, body := capture(t, `{"ok":true,"result":true}`)
			c, _ := newTestClient(t, handler)

			if err := c.SetChatMenuButton(context.Background(), 0, tt.button); err != nil {
				t.Fatalf("SetChatMenuButton: %v", err)
			}
			menu, _ := (*body)["menu_button"].(map[string]any)
			if menu["type"] != tt.want || len(menu) != 1 {
				t.Fatalf("menu_button = %v, want only type %q", menu, tt.want)
			}
		})
	}
}

func TestSetChatMenuButtonRefusesWhatItCannotSendBeforeSending(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("a menu button that cannot be sent must not reach the network")
	})
	for name, button := range map[string]MenuButton{
		"web_app without text": {Type: MenuWebApp, WebApp: &WebAppInfo{URL: "https://example.test"}},
		"web_app without app":  {Type: MenuWebApp, Text: "Open"},
		"web_app without url":  {Type: MenuWebApp, Text: "Open", WebApp: &WebAppInfo{}},
		"unknown type":         {Type: "webapp", Text: "Open", WebApp: &WebAppInfo{URL: "https://example.test"}},
	} {
		if err := c.SetChatMenuButton(context.Background(), 0, button); err == nil {
			t.Fatalf("%s: want a refusal", name)
		}
	}
}

// Both bots write the texts for each language at start.
func TestSetMyDescriptionForOneLanguage(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.SetMyDescription(context.Background(), "A guessing game with maps.", "uk"); err != nil {
		t.Fatalf("SetMyDescription: %v", err)
	}
	if *method != "setMyDescription" || (*body)["description"] != "A guessing game with maps." || (*body)["language_code"] != "uk" {
		t.Fatalf("method = %q, body = %v", *method, *body)
	}
}

func TestSetMyShortDescriptionUsesItsOwnField(t *testing.T) {
	handler, method, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.SetMyShortDescription(context.Background(), "Guess the place.", "be"); err != nil {
		t.Fatalf("SetMyShortDescription: %v", err)
	}
	if *method != "setMyShortDescription" || (*body)["short_description"] != "Guess the place." || (*body)["language_code"] != "be" {
		t.Fatalf("method = %q, body = %v", *method, *body)
	}
	if _, has := (*body)["description"]; has {
		t.Fatal("the short description must not travel as description")
	}
}

// Without a language the text is the default for every user who has none of
// their own, and that is said by leaving the parameter out.
func TestDescriptionsWithoutALanguageSendNoLanguageCode(t *testing.T) {
	handler, _, body := capture(t, `{"ok":true,"result":true}`)
	c, _ := newTestClient(t, handler)

	if err := c.SetMyDescription(context.Background(), "text", ""); err != nil {
		t.Fatalf("SetMyDescription: %v", err)
	}
	if _, has := (*body)["language_code"]; has {
		t.Fatalf("language_code = %v, want none", (*body)["language_code"])
	}
}

// An empty text is how Telegram is told to drop the description of a language,
// so it has to be on the wire as an empty string and not left out.
func TestEmptyDescriptionIsSentToRemoveIt(t *testing.T) {
	for name, set := range map[string]func(*Client) error{
		"description":       func(c *Client) error { return c.SetMyDescription(context.Background(), "", "uk") },
		"short_description": func(c *Client) error { return c.SetMyShortDescription(context.Background(), "", "uk") },
	} {
		t.Run(name, func(t *testing.T) {
			handler, _, body := capture(t, `{"ok":true,"result":true}`)
			c, _ := newTestClient(t, handler)

			if err := set(c); err != nil {
				t.Fatalf("set: %v", err)
			}
			text, has := (*body)[name]
			if !has || text != "" {
				t.Fatalf("%s = %v (present %v), want an empty string on the wire", name, text, has)
			}
		})
	}
}
