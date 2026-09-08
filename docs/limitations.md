# What this package cannot do yet

The client models what the family's bots use. This is the list of places where
that is currently too narrow, written down so nobody has to rediscover it by
reading the source.

## Missing capabilities

- **No multipart uploads.** `Call` marshals JSON, and nothing in the package
  builds a multipart body, so a file can be downloaded but not sent. Note that
  `probeSafe` accepts `sendAudio`, `sendDocument`, `sendPhoto`, `sendVideo` and
  `sendVoice`: a bot can assert those exist on the server and still have no way
  to call them. Sending media is the obvious next thing voicy will want.
- **`Probe` cannot be extended by a caller.** The safe list is unexported, and
  `Probe` refuses anything outside it. `Call` exists so a bot is never blocked
  on a release for a missing method; `Probe` has no equivalent, so naming a new
  method in `Needs` does require one.
- **`BotCommandScope` carries only a type.** The `chat`,
  `chat_administrators` and `chat_member` scopes need a chat id and sometimes a
  user id, so per-group command lists are unreachable.
- **`EditMessageText` cannot address an `inline_message_id`.** Inline-mode
  editing is out of reach until the signature changes.
- **`CallbackQuery.Message` is a value, not a pointer.** Telegram omits it for
  a callback from an inline message, and a zero value there reads as chat 0
  rather than as absence. No bot in the family sends inline messages today;
  the first one to do so has to make this a pointer, which every caller sees.
- **Downloads are invisible to the `Observer`.** On a self-hosted server a
  download is the largest and slowest thing a bot does, and it emits no event.

## Deliberate rigidity

- **`SendMessage` always sends HTML with link previews off**, and
  `SendPlainText` is a second method rather than an option. There is no way to
  ask for a link preview, MarkdownV2, `disable_notification` or
  `protect_content` without dropping to `Call` and losing the typed return. A
  third variant should become options rather than a third method.
- **`SendRichMarkdown` takes no reply or thread**, while `SendRichHTML` does.
  Asymmetric for no protocol reason.
- **`GetUpdates` exposes no `limit`, and cannot send a negative offset**, so
  Telegram's "give me the last N updates" recovery mode is unreachable.

## Sharp edges

- **An empty keyboard is not the zero value.** `&InlineKeyboardMarkup{}`
  marshals to `{"inline_keyboard":null}`, which Telegram rejects; clearing a
  keyboard needs `&InlineKeyboardMarkup{InlineKeyboard: [][]InlineKeyboardButton{}}`.
  Passing `nil` means "leave the keyboard alone".
- **`WithHTTPClient` does not enforce what it documents.** A client with a
  global `Timeout` shorter than a long poll turns every poll into four failed
  attempts, with nothing pointing at the option as the cause.
- **`WithTimeout(0)`, `WithAPIBase("")` and `WithLocalFiles("")` are ignored
  rather than refused**, so a misconfigured empty value looks like it applied.
