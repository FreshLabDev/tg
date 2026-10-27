// SPDX-License-Identifier: Apache-2.0
package tg

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
)

// Media is the one corner of the Bot API that is not a JSON API. A file
// reaches Telegram in one of three ways, and only the last of them needs a
// different kind of request:
//
//   - as a file_id Telegram already stores, or an https URL it fetches itself;
//   - as a path on the disk of a server started with --local, which opens the
//     file instead of receiving it;
//   - as bytes, which have to be uploaded as multipart/form-data.
//
// So every send below builds an ordinary JSON body until an [InputFileUpload]
// appears among its files, and a multipart one when it does. The choice is not
// the caller's to make: it follows from which [InputFile] was passed.

// InputFile is the file a send method attaches.
type InputFile interface {
	// resolve reports how the file travels to c. Either value is the parameter
	// Telegram reads, or data is the bytes to upload under filename -- never
	// both.
	resolve(c *Client) (value, filename string, data io.Reader, err error)
}

// InputFileString is a file Telegram can find on its own: a file_id it already
// stores, or an http(s) URL it fetches. It costs no upload, so it is what a
// bot re-sending known media should use.
type InputFileString struct {
	Data string
}

func (f InputFileString) resolve(*Client) (string, string, io.Reader, error) {
	if f.Data == "" {
		return "", "", nil, fmt.Errorf("empty file identifier")
	}
	return f.Data, "", nil, nil
}

// InputFileLocal names a file on the disk of a Bot API server started with
// --local, which reads it directly instead of receiving an upload. That is the
// only way to send something larger than the 50 MB an upload is capped at.
//
// The path is resolved by the server, not by this process, so nothing here can
// check that it exists; what this package can check is that the client is
// pointed at a self-hosted server at all, because Telegram's own endpoint has
// no access to any disk.
type InputFileLocal struct {
	Path string
}

func (f InputFileLocal) resolve(c *Client) (string, string, io.Reader, error) {
	if !c.IsLocalServer() {
		return "", "", nil, fmt.Errorf("a local file path can only be sent to a self-hosted Bot API server; this client talks to %s", c.apiBase)
	}
	if !filepath.IsAbs(f.Path) {
		return "", "", nil, fmt.Errorf("local file path %q must be absolute: it is resolved on the server, where this process's working directory means nothing", f.Path)
	}
	return "file://" + f.Path, "", nil, nil
}

// InputFileUpload sends bytes with the request. Data is read once, into the
// request body; see [Client.postMultipart] for what that costs.
type InputFileUpload struct {
	// Filename is what Telegram records and what a client shows on a
	// downloaded document. An empty one becomes "file", which is worse than
	// anything a caller would pick.
	Filename string
	Data     io.Reader
}

func (f InputFileUpload) resolve(*Client) (string, string, io.Reader, error) {
	if f.Data == nil {
		return "", "", nil, fmt.Errorf("upload has no data")
	}
	name := f.Filename
	if name == "" {
		name = "file"
	}
	return "", name, f.Data, nil
}

// attachment is one file carried by a multipart request.
type attachment struct {
	field    string
	filename string
	data     io.Reader
}

// attachTop resolves a top-level file parameter such as sendPhoto's "photo".
// An uploaded file becomes the multipart part named after the parameter
// itself, which is the form the Bot API documents for these; anything else
// stays an ordinary string field.
func (c *Client) attachTop(fields map[string]any, files *[]attachment, param string, file InputFile) error {
	if file == nil {
		return fmt.Errorf("%s requires a file", param)
	}
	value, filename, data, err := file.resolve(c)
	if err != nil {
		return fmt.Errorf("%s: %w", param, err)
	}
	if data != nil {
		*files = append(*files, attachment{field: param, filename: filename, data: data})
		return nil
	}
	fields[param] = value
	return nil
}

// attachInner is attachTop for a file inside an InputMedia, where an upload
// cannot be the parameter itself: the object's "media" field carries an
// attach:// reference to a part named separately.
func (c *Client) attachInner(files *[]attachment, name string, file InputFile) (string, error) {
	if file == nil {
		return "", fmt.Errorf("media item requires a file")
	}
	value, filename, data, err := file.resolve(c)
	if err != nil {
		return "", err
	}
	if data != nil {
		*files = append(*files, attachment{field: name, filename: filename, data: data})
		return "attach://" + name, nil
	}
	return value, nil
}

// CaptionOptions are the parameters every captioned send shares. The caption
// is HTML, like every other text this package sends.
type CaptionOptions struct {
	// ThreadID posts into a forum topic; zero stays in the chat's main thread.
	ThreadID int
	// ReplyTo answers a message; zero sends a standalone one.
	ReplyTo int64
	Caption string
	Markup  *InlineKeyboardMarkup
}

// VideoOptions carries what Telegram cannot learn from a file it did not
// decode. Leaving the dimensions at zero is legal and makes clients guess,
// which shows a square placeholder until the video has loaded.
type VideoOptions struct {
	CaptionOptions
	Width             int
	Height            int
	Duration          int
	SupportsStreaming bool
}

// AnimationOptions carries what Telegram cannot learn from a file it did not
// decode. Like a video, an animation uploaded without its dimensions shows a
// square placeholder until it has loaded.
type AnimationOptions struct {
	CaptionOptions
	Width    int
	Height   int
	Duration int
	// ShowCaptionAboveMedia puts the caption over the animation instead of under
	// it.
	ShowCaptionAboveMedia bool
	// HasSpoiler hides the animation under a spoiler until the user taps it.
	HasSpoiler bool
}

type AudioOptions struct {
	CaptionOptions
	Duration  int
	Performer string
	Title     string
}

type DocumentOptions struct {
	CaptionOptions
	// DisableContentTypeDetection stops Telegram from turning a document back
	// into a photo or a video, which is the whole point of sending one as a
	// document.
	DisableContentTypeDetection bool
}

func mediaFields(chatID int64, opts CaptionOptions) map[string]any {
	req := map[string]any{"chat_id": chatID}
	if opts.ThreadID > 0 {
		req["message_thread_id"] = opts.ThreadID
	}
	if opts.ReplyTo > 0 {
		req["reply_parameters"] = map[string]any{
			"message_id":                  opts.ReplyTo,
			"allow_sending_without_reply": true,
		}
	}
	if opts.Caption != "" {
		req["caption"] = opts.Caption
		req["parse_mode"] = "HTML"
	}
	if opts.Markup != nil {
		req["reply_markup"] = opts.Markup
	}
	return req
}

func (c *Client) SendPhoto(ctx context.Context, chatID int64, photo InputFile, opts CaptionOptions) (Message, error) {
	fields := mediaFields(chatID, opts)
	var files []attachment
	if err := c.attachTop(fields, &files, "photo", photo); err != nil {
		return Message{}, err
	}
	return c.sendMedia(ctx, "sendPhoto", fields, files)
}

func (c *Client) SendVideo(ctx context.Context, chatID int64, video InputFile, opts VideoOptions) (Message, error) {
	fields := mediaFields(chatID, opts.CaptionOptions)
	if opts.Width > 0 {
		fields["width"] = opts.Width
	}
	if opts.Height > 0 {
		fields["height"] = opts.Height
	}
	if opts.Duration > 0 {
		fields["duration"] = opts.Duration
	}
	if opts.SupportsStreaming {
		fields["supports_streaming"] = true
	}
	var files []attachment
	if err := c.attachTop(fields, &files, "video", video); err != nil {
		return Message{}, err
	}
	return c.sendMedia(ctx, "sendVideo", fields, files)
}

// SendAnimation sends a GIF, or an H.264/MPEG-4 AVC video without sound, which
// clients show as an animation. Telegram's own endpoint takes up to 50 MB.
//
// The returned message holds the file in Animation, and also in Document, which
// Telegram fills for backward compatibility. Animation.FileID is the one to keep
// and to send again.
func (c *Client) SendAnimation(ctx context.Context, chatID int64, animation InputFile, opts AnimationOptions) (Message, error) {
	fields := mediaFields(chatID, opts.CaptionOptions)
	if opts.Width > 0 {
		fields["width"] = opts.Width
	}
	if opts.Height > 0 {
		fields["height"] = opts.Height
	}
	if opts.Duration > 0 {
		fields["duration"] = opts.Duration
	}
	if opts.ShowCaptionAboveMedia {
		fields["show_caption_above_media"] = true
	}
	if opts.HasSpoiler {
		fields["has_spoiler"] = true
	}
	var files []attachment
	if err := c.attachTop(fields, &files, "animation", animation); err != nil {
		return Message{}, err
	}
	return c.sendMedia(ctx, "sendAnimation", fields, files)
}

func (c *Client) SendAudio(ctx context.Context, chatID int64, audio InputFile, opts AudioOptions) (Message, error) {
	fields := mediaFields(chatID, opts.CaptionOptions)
	if opts.Duration > 0 {
		fields["duration"] = opts.Duration
	}
	if opts.Performer != "" {
		fields["performer"] = opts.Performer
	}
	if opts.Title != "" {
		fields["title"] = opts.Title
	}
	var files []attachment
	if err := c.attachTop(fields, &files, "audio", audio); err != nil {
		return Message{}, err
	}
	return c.sendMedia(ctx, "sendAudio", fields, files)
}

func (c *Client) SendDocument(ctx context.Context, chatID int64, document InputFile, opts DocumentOptions) (Message, error) {
	fields := mediaFields(chatID, opts.CaptionOptions)
	if opts.DisableContentTypeDetection {
		fields["disable_content_type_detection"] = true
	}
	var files []attachment
	if err := c.attachTop(fields, &files, "document", document); err != nil {
		return Message{}, err
	}
	return c.sendMedia(ctx, "sendDocument", fields, files)
}

// InputMedia is one item of a media group, or the replacement in
// [Client.EditMessageMedia].
type InputMedia interface {
	// item returns the object without its "media" field, which the caller
	// fills in once it knows whether the file is uploaded or named.
	item() (map[string]any, InputFile)
}

// InputMediaPhoto is a photo. The caption is HTML, like every other text this
// package sends.
type InputMediaPhoto struct {
	Media   InputFile
	Caption string
	// ShowCaptionAboveMedia puts the caption over the photo instead of under it.
	ShowCaptionAboveMedia bool
	// HasSpoiler hides the photo under a spoiler until the user taps it.
	HasSpoiler bool
}

func (m InputMediaPhoto) item() (map[string]any, InputFile) {
	object := mediaItem("photo", m.Caption)
	captionPlacement(object, m.ShowCaptionAboveMedia, m.HasSpoiler)
	return object, m.Media
}

type InputMediaVideo struct {
	Media             InputFile
	Caption           string
	Width             int
	Height            int
	Duration          int
	SupportsStreaming bool
	// ShowCaptionAboveMedia puts the caption over the video instead of under it.
	ShowCaptionAboveMedia bool
	// HasSpoiler hides the video under a spoiler until the user taps it.
	HasSpoiler bool
}

func (m InputMediaVideo) item() (map[string]any, InputFile) {
	object := mediaItem("video", m.Caption)
	if m.Width > 0 {
		object["width"] = m.Width
	}
	if m.Height > 0 {
		object["height"] = m.Height
	}
	if m.Duration > 0 {
		object["duration"] = m.Duration
	}
	if m.SupportsStreaming {
		object["supports_streaming"] = true
	}
	captionPlacement(object, m.ShowCaptionAboveMedia, m.HasSpoiler)
	return object, m.Media
}

// InputMediaAnimation is a GIF or an H.264/MPEG-4 AVC video without sound. In
// [Client.EditMessageMedia] it replaces the media of a message with an
// animation. An album cannot hold one, and [Client.SendMediaGroup] refuses it.
type InputMediaAnimation struct {
	Media                 InputFile
	Caption               string
	Width                 int
	Height                int
	Duration              int
	ShowCaptionAboveMedia bool
	HasSpoiler            bool
}

func (m InputMediaAnimation) item() (map[string]any, InputFile) {
	object := mediaItem("animation", m.Caption)
	if m.Width > 0 {
		object["width"] = m.Width
	}
	if m.Height > 0 {
		object["height"] = m.Height
	}
	if m.Duration > 0 {
		object["duration"] = m.Duration
	}
	captionPlacement(object, m.ShowCaptionAboveMedia, m.HasSpoiler)
	return object, m.Media
}

type InputMediaDocument struct {
	Media                       InputFile
	Caption                     string
	DisableContentTypeDetection bool
}

func (m InputMediaDocument) item() (map[string]any, InputFile) {
	object := mediaItem("document", m.Caption)
	if m.DisableContentTypeDetection {
		object["disable_content_type_detection"] = true
	}
	return object, m.Media
}

// captionPlacement adds the two flags a photo, a video and an animation share.
func captionPlacement(object map[string]any, above, spoiler bool) {
	if above {
		object["show_caption_above_media"] = true
	}
	if spoiler {
		object["has_spoiler"] = true
	}
}

func mediaItem(kind, caption string) map[string]any {
	object := map[string]any{"type": kind}
	if caption != "" {
		object["caption"] = caption
		object["parse_mode"] = "HTML"
	}
	return object
}

// SendMediaGroup posts an album: one message group, delivered atomically, that
// clients render as a single tile of up to ten items. A group carries no
// keyboard of its own -- Telegram allows none -- so an action button has to
// live on a separate message.
func (c *Client) SendMediaGroup(ctx context.Context, chatID int64, media []InputMedia, threadID int) ([]Message, error) {
	if len(media) == 0 {
		return nil, fmt.Errorf("sendMediaGroup requires at least one item")
	}
	fields := map[string]any{"chat_id": chatID}
	if threadID > 0 {
		fields["message_thread_id"] = threadID
	}
	var files []attachment
	items := make([]map[string]any, 0, len(media))
	for i, entry := range media {
		switch entry.(type) {
		case InputMediaAnimation, *InputMediaAnimation:
			return nil, fmt.Errorf("media[%d]: an animation cannot be part of an album; send it with SendAnimation", i)
		}
		object, file := entry.item()
		// The attach name only has to be unique within this request; the index
		// makes it so and makes a captured request readable.
		value, err := c.attachInner(&files, "media"+strconv.Itoa(i), file)
		if err != nil {
			return nil, fmt.Errorf("media[%d]: %w", i, err)
		}
		object["media"] = value
		items = append(items, object)
	}
	fields["media"] = items
	var resp struct {
		OK     bool      `json:"ok"`
		Result []Message `json:"result"`
	}
	if err := c.submit(ctx, "sendMediaGroup", fields, files, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, okFalse("sendMediaGroup")
	}
	return resp.Result, nil
}

// EditMessageMedia replaces the file in a message the bot already sent. It is
// how a paged gallery moves between pages inside one message instead of
// posting a new one for every tap. A "message is not modified" answer is
// success, exactly as it is for [Client.EditMessageText].
//
// It returns nothing but the error. A caller that needs the edited message --
// the file_id of a file it just uploaded, to send it again -- uses
// [Client.EditMessageMediaMessage].
func (c *Client) EditMessageMedia(ctx context.Context, chatID, messageID int64, media InputMedia, markup *InlineKeyboardMarkup) error {
	var resp struct {
		OK bool `json:"ok"`
	}
	err := c.editMessageMedia(ctx, chatID, messageID, media, markup, &resp)
	if IsMessageNotModified(err) {
		return nil
	}
	return err
}

// EditMessageMediaMessage is [Client.EditMessageMedia] that also returns the
// edited message, which is where Telegram reports the file_id of a file the
// edit uploaded: in Photo, Video, Animation or Document, whichever the new media
// is. The caption goes with the media in the same call, as HTML, with the
// placement and spoiler options on [InputMediaPhoto], [InputMediaVideo] and
// [InputMediaAnimation].
//
// A "message is not modified" answer is returned as the error, unlike
// [Client.EditMessageMedia]: there is no message to return, and a zero Message
// with a nil error would read as an edit that succeeded and carries no file.
// [IsMessageNotModified] tells it apart.
//
// Editing an inline message returns true instead of a message, so it is not
// supported here: the call needs a chat and a message id.
func (c *Client) EditMessageMediaMessage(ctx context.Context, chatID, messageID int64, media InputMedia, markup *InlineKeyboardMarkup) (Message, error) {
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.editMessageMedia(ctx, chatID, messageID, media, markup, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse("editMessageMedia")
	}
	return resp.Result, nil
}

func (c *Client) editMessageMedia(ctx context.Context, chatID, messageID int64, media InputMedia, markup *InlineKeyboardMarkup, out any) error {
	if media == nil {
		return fmt.Errorf("editMessageMedia requires media")
	}
	fields := map[string]any{"chat_id": chatID, "message_id": messageID}
	object, file := media.item()
	var files []attachment
	value, err := c.attachInner(&files, "media", file)
	if err != nil {
		return err
	}
	object["media"] = value
	fields["media"] = object
	if markup != nil {
		fields["reply_markup"] = markup
	}
	return c.submit(ctx, "editMessageMedia", fields, files, out)
}

// EditMessageReplyMarkup changes only a message's buttons. It is what removes
// an action that has become unsafe or already ran, without disturbing the
// content the user is looking at.
func (c *Client) EditMessageReplyMarkup(ctx context.Context, chatID, messageID int64, markup *InlineKeyboardMarkup) error {
	req := map[string]any{"chat_id": chatID, "message_id": messageID}
	if markup != nil {
		req["reply_markup"] = markup
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	err := c.post(ctx, "editMessageReplyMarkup", req, &resp)
	if IsMessageNotModified(err) {
		return nil
	}
	return err
}

func (c *Client) sendMedia(ctx context.Context, method string, fields map[string]any, files []attachment) (Message, error) {
	var resp struct {
		OK     bool    `json:"ok"`
		Result Message `json:"result"`
	}
	if err := c.submit(ctx, method, fields, files, &resp); err != nil {
		return Message{}, err
	}
	if !resp.OK {
		return Message{}, okFalse(method)
	}
	return resp.Result, nil
}

// submit sends a request that may or may not carry files. JSON is preferred
// whenever nothing is uploaded: it is cheaper, and it is the only body a
// parameterless call can send without upsetting a self-hosted server.
func (c *Client) submit(ctx context.Context, method string, fields map[string]any, files []attachment, out any) error {
	if len(files) == 0 {
		return c.post(ctx, method, fields, out)
	}
	return c.postMultipart(ctx, method, fields, files, out)
}
