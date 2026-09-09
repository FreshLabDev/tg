# Changelog

All notable tg changes are documented here.

The `## <tag>` section of this file *is* the GitHub Release body: the release
workflow copies it verbatim and refuses a tag that has no section. Write it
for whoever has to decide whether to upgrade.

See [`docs/versioning.md`](docs/versioning.md) for what the numbers mean and
[`docs/releases.md`](docs/releases.md) for how a release is published.

## Unreleased

Use this section for changes that are merged but not released yet.

## v0.1.0 - 2026-09-09

The first stable release of the 0.1.0 line, summarising it rather than only the
delta since alpha.1.

tg gained the surface searchy needed to leave its third-party client: media
sends, inline mode, link-preview control and the chat and action constants that
went with them. Nothing that already existed moved, so the three bots already on
this module are unaffected by any of it.

Below, the entries added after alpha.1 was tagged. The media and inline work is
described in the alpha.1 section.

### Added

- `docs/releases.md` gained a **Deploying** section, and `AGENTS.md` points at it.
  Releasing was documented; deploying was not, in any repository in the family —
  the process stopped at "deploy it" and never said how. That gap mattered more
  after the stacks moved from building on the host to pulling a published image,
  because the procedure changed on the same day. The section names this stack's
  host directory, its env file, the variable that selects the image, the networks
  it needs, and what a rollback actually is.

## v0.1.0-alpha.1 - 2026-09-09

The surface searchy needs to leave its third-party client. Nothing that
existed moved, so every bot already on tg is unaffected; the version says
`0.1.0` rather than `0.0.2` because the additions are notable, and carries
`alpha` because no bot has yet run on them against live Telegram.

### Added

- **Media sends.** `SendPhoto`, `SendVideo`, `SendAudio`, `SendDocument` and
  `SendMediaGroup`, plus `EditMessageMedia` and `EditMessageReplyMarkup`. A file
  is an `InputFile`, and which of the three shapes it takes decides the request
  on its own: `InputFileString` for a `file_id` or an https URL Telegram fetches
  itself, `InputFileLocal` for a path on the disk of a server started with
  `--local`, and `InputFileUpload` for bytes — only the last builds a multipart
  body. A local path sent to Telegram's own endpoint is refused before the
  request, because the failure it would otherwise produce ("wrong file
  identifier") points nowhere near the cause.
- **Inline mode.** `AnswerInlineQuery`, the `InlineQuery` and
  `ChosenInlineResult` update kinds, and the two result cards the family uses:
  `InlineQueryResultPhoto` and `InlineQueryResultArticle` with
  `InputTextMessageContent`. Answering with an empty result list is now
  expressible, and `results` is always sent, because an omitted parameter is a
  400 rather than "nothing found".
- `LinkPreviewOptions` as a type, `SendTextWithPreview` for the one case where a
  bot relays text whose author wanted the preview, and `AnswerCallbackQueryURL`
  for a button that opens a `t.me` deep link with no intermediate message.
- `ChatAction*` and `Chat*` constants, so a bot branching on chat type or
  announcing an upload no longer spells the strings out at every call site.
- `InlineKeyboardButton.SwitchInlineQueryCurrentChat`. It is a pointer because
  the empty string is the useful value — "open inline search with nothing
  typed" — and `omitempty` on a plain string would drop exactly that button.
- `editMessageMedia` and `sendMediaGroup` are now probe-safe, so a bot that
  pages a gallery or posts an album can refuse to start on a server that cannot.

  This is why the whole set landed: searchy was the one bot still on a
  third-party library, and it stayed there because tg could not send a photo or
  answer an inline query. Everything above is additive — no existing signature
  moved.

### Fixed

- `EditEphemeralMessageText` suppresses Telegram's own link preview, which it
  alone among the text sends did not. The About panel every bot is growing
  carries a repository link, so editing one in place rendered a preview card
  under it — in a group, where an ephemeral panel exists precisely to be the
  quiet option. `SendEphemeralMessage` already did this; the rich sends do not,
  ephemeral or not, which is consistent between them and left alone here.

### Changed

- One versioning and release document for the whole family. `docs/versioning.md`
  and `docs/releases.md` are now byte-identical across every Asterfield
  repository apart from two clearly marked sections: this repository's own
  version line, and the surface where a change here breaks something. They spell
  out what each of the three numbers means, what the `-alpha.N` suffix counts,
  when alpha becomes beta and when it is legitimate to skip to rc or run a
  pre-release in production.
- **Pre-releases are now tagged on `dev`, not `main`.** Only stable versions are
  tagged on `main`, on the merge commit from `dev`, so `main` answers exactly one
  question: what is in production. The test bot runs `dev`, the production bot
  runs `main`. `release.yml` enforces this and refuses a tag on the wrong branch.
  Earlier pre-releases in this repository were tagged on `main` under the
  previous rule; they are left as they are.
- The document is explicit that tg's major number is not a judgement call: Go
  gives it a hard meaning, since a `v2` module has a different import path. A
  change that removes or re-signs an exported function is major whatever it
  feels like.

### Added

- `release.yml`. Every tag from `v0.0.1-alpha.1` onward existed and was
  fetchable through the module proxy, but none of them had a GitHub Release, so
  the only way to learn what a bump changed was to open the changelog in
  another tab. The workflow verifies the tag is on the branch its channel
  publishes from, re-runs gofmt, vet, the race suite and govulncheck, and
  publishes the release from the matching changelog section. The module proxy
  caches a tag permanently on first fetch, which is why the branch check runs
  before anything else.

## v0.0.1-alpha.7 - 2026-09-08

### Fixed

- `Event.Probe` marks the requests a capability check makes. A probe calls a
  method with an empty body on purpose and is answered with a parameter error,
  so an observer counting failures recorded one phantom incident per probed
  method on every start -- visible in voicy's metrics as two Telegram errors
  after a restart that had served no traffic yet.

## v0.0.1-alpha.6 - 2026-09-08

What three reviews of the first day's code found. Two of these were leaks.

### Security

- The bot token no longer escapes through an API error. Telegram never echoes
  the request, but a proxy or a sidecar answering 404 or 502 in its place
  quotes the request URI -- which contains the token -- and that string reached
  log lines and, in branchy, a database column. Descriptions and raw bodies are
  now redacted like URLs are.
- The token no longer escapes through a wrapped `fs.PathError` either. A local
  server names each bot's directory after its token, so every local-file error
  carried it in `PathError.Path` even though the message itself was clean. The
  error keeps its identity -- `errors.Is(err, fs.ErrNotExist)` still works --
  with the path scrubbed.
- Symlinks can no longer escape `<filesRoot>/<token>`. Cleaning a path stops a
  `..` and nothing stopped a link, on a directory written by another process
  and shared with every other bot. Both sides are resolved before comparison.

### Fixed

- A 2xx answer carrying `ok=false` is an `*APIError` again, with its error
  code, description, retry_after and body. Telegram does not answer that way,
  but a proxy does -- and callers classify failures by type, so it was arriving
  as an untyped error that skipped every recovery path. Classification now
  reads Telegram's own code when the HTTP status does not carry it, which also
  fixes `answerCallbackQuery`, `setMyCommands` and `editMessageText`, none of
  which checked `ok` at all.
- `ErrUnexpectedResult` separates "the call failed" from "the call was made and
  the answer is unreadable". A bot whose notification was delivered should not
  queue it again because the receipt did not parse; a bot that needs the new
  message's id genuinely cannot continue. Such an answer is also no longer
  retried -- it will not parse differently the second time -- and it now names
  the method.
- A failed download no longer leaves a truncated file behind. `os.Create`
  truncates before the size is known, and a caller that finds a file where an
  error was reported will read it.
- `Preflight` waits through the failures a starting server actually produces.
  It gave up on any API answer, which meant a 502 from a proxy in front of a
  booting server was fatal; only a refused connection was ever waited out.
- `Needs.Wait` bounds the whole check rather than only the pauses inside it.
  With the retries inside `getMe`, a 30-second budget could run for minutes,
  and an operator sizes a restart policy on that number.
- A probe survives a dropped connection. An ordinary POST is not replayed
  because a lost answer may still have been acted on, but a probe carries an
  empty body, so one reset connection was reading as "the server lacks this
  method".
- `Probe` refuses to guess when the server answers 401, 403, or a 404 that is
  not Telegram's. Those say the server never looked at the method, and
  reporting it present defeats the point of a guard that exists to refuse.
- A POST that is asked to wait longer than its caller can afford now returns
  both facts: what Telegram answered and that the wait was cut short. It used
  to return only the context error, losing the 429 and its retry_after.
- The JSON contract matches the API: fields Telegram always sends marshal at
  their zero value -- an accidentally empty button label was being sent as a
  button with no label at all -- and an `Update` no longer marshals three
  nulls. `File.file_unique_id`, `Video.width`/`height`,
  `ChatMemberUpdated.date`/`old_chat_member` and `CallbackQuery.chat_instance`
  are modeled; `CallbackQuery.data` is optional, as it is in the API.

### Changed

- `Event.Retried` became `Event.Retryable`. It was computed from the error
  alone and claimed another attempt would follow, which was untrue for a 5xx on
  a POST and for the last attempt of an exhausted GET.

### Documentation

- `docs/limitations.md` lists what the package cannot express yet -- multipart
  uploads, per-chat command scopes, inline-message editing, an extensible probe
  list -- so the next consumer meets them on paper rather than in the source.

## v0.0.1-alpha.5 - 2026-09-08

### Added

- `SendPlainText`, a send with no parse mode. It is the delivery of last
  resort: HTML that Telegram rejects fails the whole message, and branchy's
  notification outbox would rather deliver an unformatted notification than
  none. Every other send in this package sets HTML, which is what made this
  worth a method rather than a flag.

## v0.0.1-alpha.4 - 2026-09-08

### Changed

- Struct tags now mirror Telegram's own required/optional split: a field the
  API always sends is always marshaled, an optional one only when set. A
  message re-encoded for an audit record used to come out mostly nulls this
  package had invented.

## v0.0.1-alpha.3 - 2026-09-08

### Added

- `MessageEntity`, with `Message.Entities` and `Message.CaptionEntities`.
  Telegram's own markup of a text -- the bold run, the link, the code span --
  is protocol, and a bot that turns it back into Markdown cannot work without
  it. Offsets are UTF-16 code units, which is the part worth knowing.

## v0.0.1-alpha.2 - 2026-09-08

What the second consumer needed. makeitMD keeps an audit trail of exactly what
Telegram said and renders Markdown written by people, and neither survived the
first cut of this package.

### Added

- `Message.Raw` carries the message exactly as Telegram sent it. This package
  models the fields the family uses and no more, so Raw is how a bot reaches
  one that is not modeled yet, and how a bot that stores an audit trail keeps
  what arrived rather than a re-encoding of our struct.
- `APIError.Response` keeps the error body verbatim, for the same reason.
- `RichOption` and `WithEntityDetection()`. Rich payloads skip Telegram's
  entity detection by default, which is right for generated text full of
  digits and words that are not links -- and wrong for Markdown a person
  wrote, where a bare URL is meant to become one. The rich senders now take
  options.

## v0.0.1-alpha.1 - 2026-09-08

The first cut: the transport that had been living in three bots at once, plus
the two things none of those copies had. Alpha until a real bot runs on it —
voicy is first, and nothing here has met live Telegram credentials yet.

### Added

- `Client` over `net/http` with per-attempt deadlines, retries for idempotent
  GETs, `Retry-After` handling, and POSTs that are replayed only when Telegram
  asks — a lost response must never duplicate a message.
- Token redaction on every path out of the package, for errors and for the
  file paths a `--local` server produces, which are named after the token.
- `Observer` and `Event` for metrics, and `*slog.Logger` for logging, so the
  module keeps an empty `require` block and no bot inherits another's choice.
- `Probe`, `Require` and `Preflight`. A Bot API server reports no version
  anywhere, but answers `404 method not found` for a method it lacks and a
  parameter error for one it has. A bot now refuses to start against a server
  that cannot serve it, instead of polling happily and answering nothing.
- `WithLocalFiles` for a server started with `--local`: media is read from
  disk under `<root>/<token>` and removed afterwards, since such a server
  never reclaims files. There is no HTTP fallback, because that route answers
  404 by design.
- Rich messages (`sendRichMessage`, HTML and Markdown) and the Bot API 10.3
  ephemeral contract, including editing an ephemeral placeholder — the only
  way to deliver long output privately once the reply window has closed.
- Methods the family uses: getMe, getUpdates, setMyCommands, deleteWebhook,
  sendMessage, editMessageText, deleteMessage, answerCallbackQuery,
  sendChatAction, getChatMember, getFile, downloads, logOut. Anything else
  goes through `Call` without waiting for a release.
- `SplitText` for plain text at Telegram's 4096-rune limit.
- `BotAPI` declares the targeted Bot API version; CI compares it with
  Telegram's published one so falling behind is noticed by a job, not by a
  user whose message went unanswered.
