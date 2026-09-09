# What this package cannot do yet

The client models what the family's bots use. This is the list of places where
that is currently too narrow, written down so nobody has to rediscover it by
reading the source.

## Missing capabilities

- **An upload is held whole in memory.** `postMultipart` assembles the body in
  a buffer rather than streaming it through an `io.Pipe`, because a POST is
  replayed when Telegram answers `Retry-After` and a pipe cannot be rewound. So
  a send is bounded by what the process can hold; anything genuinely large
  belongs on a self-hosted server, where `InputFileLocal` hands over a path
  instead of bytes.
- **`sendVoice`, `sendVideoNote` and `sendAnimation` have no method.**
  `probeSafe` accepts `sendVoice`, so a bot can assert it exists on the server
  and still have to reach it through `Call`.
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
  rather than as absence. Searchy does send inline messages, but only ever with
  URL buttons, so no callback can arrive without a message today; a bot that
  puts `callback_data` on an inline result has to make this a pointer first,
  which every caller sees. Until then, `cq.Message.MessageID == 0` is the
  check.
- **Downloads are invisible to the `Observer`.** On a self-hosted server a
  download is the largest and slowest thing a bot does, and it emits no event.

## Deliberate rigidity

- **`SendMessage` always sends HTML with link previews off**, and
  `SendPlainText` and `SendTextWithPreview` are further methods rather than
  options. There is no way to ask for MarkdownV2, `disable_notification` or
  `protect_content` without dropping to `Call` and losing the typed return. The
  next variant should become options rather than a fourth method.
- **A caption is always HTML, and an inline result's caption always declares
  it.** A caller sending a caption assembled from user-controlled text has to
  escape it, exactly as it would for `SendMessage`.
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
