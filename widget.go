// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
)

// Widgets (Bot API 10.4) are small HTML documents that live inside a message:
// fixed code the bot sends once, plus state only the bot changes. The guide is
// https://core.telegram.org/bots/widgets. What decides whether a send works:
//
//   - In production a widget goes to a private chat only, and arrives with
//     protected content, so it cannot be forwarded. Groups, inline results
//     and forwards work on the test server only, for now.
//   - The code cannot be replaced. What changes is two strings of up to 1024
//     bytes each: ChatData belongs to one message, PublicData is shown
//     wherever the widget is, and for a shared widget it is one value for
//     every message that shows it.
//   - The widget has no network. Its one way out is a query of up to 64
//     bytes, which reaches the bot as [Update.WidgetQuery] (or
//     [Update.SharedWidgetQuery] from a forward) only when the widget was sent
//     with AllowQueries. The bot answers it within 15 seconds; the answer goes
//     to the widget that asked and changes no state.
//   - A bot that names its updates in [WithAllowedUpdates] must name
//     "widget_query" and "shared_widget_query", or no query arrives.
//   - Edits of the data show no "edited" mark and notify nobody, but each one
//     counts against the bot's sending limits like a message. A shared
//     widget takes at most one edit a second.

// Widget limits, in bytes.
const (
	MaxWidgetHTMLBytes           = 32768
	MaxWidgetCompressedHTMLBytes = 4096
	MaxWidgetDataBytes           = 1024
)

// Widget is the widget a message carries.
type Widget struct {
	// ID names a shared widget for [Client.SendSharedWidget]; it is empty
	// for a widget that is not shared.
	ID string `json:"id,omitempty"`
	// Width and Height are in logical pixels. Zero width fills the bubble,
	// and zero height leaves it to the content.
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	HTML       string `json:"html"`
	PublicData string `json:"public_data,omitempty"`
	ChatData   string `json:"chat_data,omitempty"`
}

// WidgetOptions is a widget to send. The caption is HTML, like every other
// text this package sends.
type WidgetOptions struct {
	// HTML is the whole widget: one document, its styles and script inline,
	// images as data: URLs. At most [MaxWidgetHTMLBytes], and at most
	// [MaxWidgetCompressedHTMLBytes] once zlib compresses it at level 9,
	// which [WidgetCompressedSize] estimates.
	HTML string
	// Width is clamped by the app to 200 and the width of the bubble; zero
	// fills the bubble. Height is clamped to 50–600; zero leaves it free.
	Width, Height int
	// PublicData and ChatData are the widget's state, at most
	// [MaxWidgetDataBytes] each. The server does not read them; JSON with a
	// revision number lets the widget ignore an older value that arrives late.
	PublicData, ChatData string
	// AllowQueries lets the widget ask the bot. Without it every request the
	// widget makes fails, and no wallet is offered to it.
	AllowQueries bool
	// IsShared creates a shared widget: one PublicData for every message that
	// shows it, its id in the sent message's [Widget.ID].
	IsShared bool

	Caption               string
	ShowCaptionAboveMedia bool
	ThreadID              int
	ReplyTo               int64
	Markup                *InlineKeyboardMarkup
	DisableNotification   bool
	ProtectContent        bool
	MessageEffectID       string
}

// SharedWidgetOptions sends again a shared widget the bot created. The code
// and PublicData are the widget's; only ChatData is this message's.
type SharedWidgetOptions struct {
	ChatData              string
	Caption               string
	ShowCaptionAboveMedia bool
	ThreadID              int
	ReplyTo               int64
	Markup                *InlineKeyboardMarkup
	DisableNotification   bool
	ProtectContent        bool
	MessageEffectID       string
}

// WidgetCompressedSize is the size of html compressed by zlib at level 9,
// the measure of the 4096-byte limit. Go's zlib is not the one Telegram runs,
// so near the limit the two may disagree by a few bytes; a bot that tests its
// widget against this keeps a margin.
func WidgetCompressedSize(html string) int {
	var buf bytes.Buffer
	w, _ := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	_, _ = w.Write([]byte(html))
	_ = w.Close()
	return buf.Len()
}

func checkWidgetData(method string, data map[string]string) error {
	for name, v := range data {
		if len(v) > MaxWidgetDataBytes {
			return fmt.Errorf("%s: %s is %d bytes, more than %d", method, name, len(v), MaxWidgetDataBytes)
		}
	}
	return nil
}

// SendWidget sends a message with a widget. An empty HTML, an HTML over
// [MaxWidgetHTMLBytes] or data over [MaxWidgetDataBytes] is refused before
// anything is sent; the compressed size is left to Telegram, which measures it
// with its own zlib.
func (c *Client) SendWidget(ctx context.Context, chatID int64, opts WidgetOptions) (Message, error) {
	switch n := len(opts.HTML); {
	case n == 0:
		return Message{}, fmt.Errorf("sendWidget requires html")
	case n > MaxWidgetHTMLBytes:
		return Message{}, fmt.Errorf("sendWidget: html is %d bytes, more than %d", n, MaxWidgetHTMLBytes)
	}
	if err := checkWidgetData("sendWidget", map[string]string{"public_data": opts.PublicData, "chat_data": opts.ChatData}); err != nil {
		return Message{}, err
	}
	req := widgetSendFields(chatID, opts.ThreadID, opts.ReplyTo, opts.Caption, opts.ShowCaptionAboveMedia,
		opts.Markup, opts.DisableNotification, opts.ProtectContent, opts.MessageEffectID)
	req["html"] = opts.HTML
	if opts.Width > 0 {
		req["width"] = opts.Width
	}
	if opts.Height > 0 {
		req["height"] = opts.Height
	}
	if opts.PublicData != "" {
		req["public_data"] = opts.PublicData
	}
	if opts.ChatData != "" {
		req["chat_data"] = opts.ChatData
	}
	if opts.AllowQueries {
		req["allow_queries"] = true
	}
	if opts.IsShared {
		req["is_shared"] = true
	}
	return c.sendWidget(ctx, "sendWidget", req)
}

// SendSharedWidget sends again the shared widget widgetID, which the bot
// created with [WidgetOptions.IsShared].
func (c *Client) SendSharedWidget(ctx context.Context, chatID int64, widgetID string, opts SharedWidgetOptions) (Message, error) {
	if widgetID == "" {
		return Message{}, fmt.Errorf("sendSharedWidget requires a widget_id")
	}
	if err := checkWidgetData("sendSharedWidget", map[string]string{"chat_data": opts.ChatData}); err != nil {
		return Message{}, err
	}
	req := widgetSendFields(chatID, opts.ThreadID, opts.ReplyTo, opts.Caption, opts.ShowCaptionAboveMedia,
		opts.Markup, opts.DisableNotification, opts.ProtectContent, opts.MessageEffectID)
	req["widget_id"] = widgetID
	if opts.ChatData != "" {
		req["chat_data"] = opts.ChatData
	}
	return c.sendWidget(ctx, "sendSharedWidget", req)
}

func widgetSendFields(chatID int64, threadID int, replyTo int64, caption string, above bool, markup *InlineKeyboardMarkup, silent, protect bool, effect string) map[string]any {
	req := map[string]any{"chat_id": chatID}
	if threadID > 0 {
		req["message_thread_id"] = threadID
	}
	if replyTo > 0 {
		req["reply_parameters"] = map[string]any{"message_id": replyTo, "allow_sending_without_reply": true}
	}
	if caption != "" {
		req["caption"] = caption
		req["parse_mode"] = "HTML"
	}
	if above {
		req["show_caption_above_media"] = true
	}
	if markup != nil {
		req["reply_markup"] = markup
	}
	if silent {
		req["disable_notification"] = true
	}
	if protect {
		req["protect_content"] = true
	}
	if effect != "" {
		req["message_effect_id"] = effect
	}
	return req
}

func (c *Client) sendWidget(ctx context.Context, method string, req map[string]any) (Message, error) {
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.post(ctx, method, req, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse(method)
	}
	return resp.Result, nil
}

// WidgetData is a change to a widget's state. A nil field is left as it is;
// a pointer to "" removes the value, and the widget reads null.
type WidgetData struct {
	PublicData *string
	ChatData   *string
}

func (d WidgetData) fields(method string, req map[string]any) error {
	if d.PublicData == nil && d.ChatData == nil {
		return fmt.Errorf("%s: nothing to change", method)
	}
	data := map[string]string{}
	if d.PublicData != nil {
		req["public_data"] = *d.PublicData
		data["public_data"] = *d.PublicData
	}
	if d.ChatData != nil {
		req["chat_data"] = *d.ChatData
		data["chat_data"] = *d.ChatData
	}
	return checkWidgetData(method, data)
}

// EditMessageWidgetData changes the state of the widget in a message the bot
// sent. For a shared widget PublicData changes in every message that shows
// it, at most once a second. The open widget gets the change at once.
func (c *Client) EditMessageWidgetData(ctx context.Context, chatID, messageID int64, data WidgetData) error {
	req := map[string]any{"chat_id": chatID, "message_id": messageID}
	if err := data.fields("editMessageWidgetData", req); err != nil {
		return err
	}
	return c.postTrue(ctx, "editMessageWidgetData", req)
}

// EditInlineMessageWidgetData does what [Client.EditMessageWidgetData] does
// for a widget the bot sent in inline mode.
func (c *Client) EditInlineMessageWidgetData(ctx context.Context, inlineMessageID string, data WidgetData) error {
	if inlineMessageID == "" {
		return fmt.Errorf("editMessageWidgetData requires an inline_message_id")
	}
	req := map[string]any{"inline_message_id": inlineMessageID}
	if err := data.fields("editMessageWidgetData", req); err != nil {
		return err
	}
	return c.postTrue(ctx, "editMessageWidgetData", req)
}

// EditSharedWidgetData replaces the PublicData of a shared widget, everywhere
// it is shown, at most once a second; "" removes it. Clients refresh a shared
// widget on their own schedule: cacheTime, from 5 to 3600 seconds, sets it,
// and zero keeps the current one, 30 seconds unless changed.
func (c *Client) EditSharedWidgetData(ctx context.Context, widgetID, publicData string, cacheTime int) error {
	switch {
	case widgetID == "":
		return fmt.Errorf("editSharedWidgetData requires a widget_id")
	case cacheTime != 0 && (cacheTime < 5 || cacheTime > 3600):
		return fmt.Errorf("editSharedWidgetData: cache_time %d is outside 5–3600", cacheTime)
	}
	if err := checkWidgetData("editSharedWidgetData", map[string]string{"public_data": publicData}); err != nil {
		return err
	}
	req := map[string]any{"widget_id": widgetID, "public_data": publicData}
	if cacheTime != 0 {
		req["cache_time"] = cacheTime
	}
	return c.postTrue(ctx, "editSharedWidgetData", req)
}

// AnswerWidgetQuery answers a query from a widget, within 15 seconds. data,
// up to [MaxWidgetDataBytes], goes to the widget that asked as the result of
// its request and nowhere else; it changes no state, which is what the
// EditWidgetData methods are for. For a wallet query it is the transaction
// for the widget to send.
func (c *Client) AnswerWidgetQuery(ctx context.Context, widgetQueryID, data string) error {
	if widgetQueryID == "" {
		return fmt.Errorf("answerWidgetQuery requires a widget_query_id")
	}
	if err := checkWidgetData("answerWidgetQuery", map[string]string{"data": data}); err != nil {
		return err
	}
	req := map[string]any{"widget_query_id": widgetQueryID}
	if data != "" {
		req["data"] = data
	}
	return c.postTrue(ctx, "answerWidgetQuery", req)
}

func (c *Client) postTrue(ctx context.Context, method string, req map[string]any) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := c.post(ctx, method, req, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return okFalse(method)
	}
	return nil
}

// WidgetQuery is a request from a widget in a message the bot sent, directly
// or in inline mode. Each person sends at most five in five seconds to the
// bot from all its widgets.
type WidgetQuery struct {
	ID   string `json:"id"`
	From User   `json:"from"`
	// Chat and MessageID are the message with the widget; both are absent
	// for an inline message, which has InlineMessageID instead.
	Chat            *Chat  `json:"chat,omitempty"`
	MessageID       int64  `json:"message_id,omitempty"`
	InlineMessageID string `json:"inline_message_id,omitempty"`
	ChatInstance    string `json:"chat_instance"`
	// ChatType is set for an inline message only: "sender", "private",
	// "group", "supergroup" or "channel".
	ChatType string          `json:"chat_type,omitempty"`
	Type     WidgetQueryType `json:"type"`
	// Data is what the widget sent, 1–64 bytes.
	Data string `json:"data"`
	// WidgetID is set when the widget is shared.
	WidgetID string `json:"widget_id,omitempty"`
}

// SharedWidgetQuery is a request from a forward of a shared widget the bot
// created. The bot learns who asked and which widget, and nothing about the
// chat it was forwarded to.
type SharedWidgetQuery struct {
	ID       string          `json:"id"`
	From     User            `json:"from"`
	Type     WidgetQueryType `json:"type"`
	Data     string          `json:"data"`
	WidgetID string          `json:"widget_id"`
}

// WidgetQueryType is Telegram's union of query kinds, flattened as
// [ReactionType] is: Type says which of the other fields is set.
//
//   - "data" is an ordinary request. WalletCard is set when the widget asked
//     with the wallet, and is the person's TON wallet card.
//   - "sign_data" carries SignData, JSON signed by Wallet for the domain
//     bot<bot_id>.
//   - "sent_transaction" says Wallet sent a transaction whose msg_hash is
//     MsgHash. It is a claim to check on chain, not a payment received.
type WidgetQueryType struct {
	Type       string             `json:"type"`
	WalletCard *TonWalletCard     `json:"wallet_card,omitempty"`
	Wallet     *WidgetQueryWallet `json:"wallet,omitempty"`
	SignData   string             `json:"sign_data,omitempty"`
	MsgHash    string             `json:"msg_hash,omitempty"`
}

// Widget query kinds, as Telegram reports them in [WidgetQueryType.Type].
const (
	WidgetQueryData            = "data"
	WidgetQuerySignData        = "sign_data"
	WidgetQuerySentTransaction = "sent_transaction"
)

// WidgetQueryWallet is the TON wallet that signed data or sent a transaction.
// Amount is the balance in nanotons, as a decimal string.
type WidgetQueryWallet struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
	// PublicKey is hex and StateInit base64; both are empty when the key is
	// not known.
	PublicKey string `json:"public_key,omitempty"`
	StateInit string `json:"state_init,omitempty"`
}

// TonWalletCard is the wallet card a person shows on their profile. Amount is
// the balance in nanotons, as a decimal string. The NFT stickers on the card
// are not modeled.
type TonWalletCard struct {
	Address              string `json:"address"`
	Amount               string `json:"amount"`
	BackgroundNFTAddress string `json:"background_nft_address,omitempty"`
}

// InputWidgetMessageContent sends a new widget as an inline result. Telegram
// does not take it in production yet, only on the test server. With IsShared
// every message sent from the result is a widget of its own, whose id the bot
// learns only from that widget's queries.
type InputWidgetMessageContent struct {
	HTML                  string `json:"html"`
	Width                 int    `json:"width,omitempty"`
	Height                int    `json:"height,omitempty"`
	PublicData            string `json:"public_data,omitempty"`
	ChatData              string `json:"chat_data,omitempty"`
	Caption               string `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool   `json:"show_caption_above_media,omitempty"`
	AllowQueries          bool   `json:"allow_queries,omitempty"`
	IsShared              bool   `json:"is_shared,omitempty"`
}

func (InputWidgetMessageContent) inputMessageContent() {}

func (c InputWidgetMessageContent) MarshalJSON() ([]byte, error) {
	type content InputWidgetMessageContent
	return json.Marshal(struct {
		ParseMode string `json:"parse_mode,omitempty"`
		content
	}{ParseMode: htmlIfAny(c.Caption), content: content(c)})
}

// InputSharedWidgetMessageContent sends an existing shared widget as an
// inline result. Like [InputWidgetMessageContent], test server only for now.
type InputSharedWidgetMessageContent struct {
	WidgetID              string `json:"widget_id"`
	ChatData              string `json:"chat_data,omitempty"`
	Caption               string `json:"caption,omitempty"`
	ShowCaptionAboveMedia bool   `json:"show_caption_above_media,omitempty"`
}

func (InputSharedWidgetMessageContent) inputMessageContent() {}

func (c InputSharedWidgetMessageContent) MarshalJSON() ([]byte, error) {
	type content InputSharedWidgetMessageContent
	return json.Marshal(struct {
		ParseMode string `json:"parse_mode,omitempty"`
		content
	}{ParseMode: htmlIfAny(c.Caption), content: content(c)})
}

func htmlIfAny(caption string) string {
	if caption == "" {
		return ""
	}
	return "HTML"
}
