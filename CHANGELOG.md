# Changelog

All notable tg changes are documented here.

tg uses SemVer-style versions. This first line starts at `v0.0.1`. Alpha,
beta, and rc tags are reserved for changes that still need live validation.

## Unreleased

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
