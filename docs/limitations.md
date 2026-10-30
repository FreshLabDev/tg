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
- **`sendVoice` and `sendVideoNote` have no method.**
  `probeSafe` accepts `sendVoice`, so a bot can assert it exists on the server
  and still have to reach it through `Call`.
- **`Probe` cannot be extended by a caller.** The safe list is unexported, and
  `Probe` refuses anything outside it. `Call` exists so a bot is never blocked
  on a release for a missing method; `Probe` has no equivalent, so naming a new
  method in `Needs` does require one.
- **`BotCommandScope` carries only a type.** The `chat`,
  `chat_administrators` and `chat_member` scopes need a chat id and sometimes a
  user id, so per-group command lists are unreachable.
- **An inline message's rich text cannot be edited.** `EditInlineMessageText`
  sends HTML text; `rich_message` on an inline message goes through `Call`.
- **`InlineAnswer.CacheTime` cannot ask for no cache.** Zero means Telegram's
  default of 300 seconds, so the shortest cache a bot can ask for is one
  second.

## Deliberate rigidity

- **`SendMessage` always sends HTML with link previews off**, and
  `SendPlainText` and `SendTextWithPreview` are further methods rather than
  options. There is no way to ask for MarkdownV2, `disable_notification`,
  `protect_content` or a message effect on a text without dropping to `Call`
  and losing the typed return. The captioned sends have the three in
  `CaptionOptions`; the next text variant should become options the same way
  rather than a fourth method.
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
