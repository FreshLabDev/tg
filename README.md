<h1 align="center">tg</h1>

<p align="center"><strong>One Telegram Bot API client for the FreshLab bots.</strong><br/>Standard library only, local Bot API server included, and it refuses to start against a server that cannot serve it.</p>

<p align="center">
  <a href="docs/versioning.md"><img src="https://img.shields.io/badge/version-v0.0.1--alpha.1-26A5E4?style=for-the-badge&labelColor=0f172a" alt="version"></a>
  <a href="go.mod"><img src="https://img.shields.io/badge/go-1.26-00ADD8?style=for-the-badge&logo=go&logoColor=white&labelColor=0f172a" alt="go version"></a>
  <a href="https://core.telegram.org/bots/api"><img src="https://img.shields.io/badge/bot%20api-10.3-26A5E4?style=for-the-badge&labelColor=0f172a" alt="Bot API 10.3"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-334155?style=for-the-badge&labelColor=0f172a" alt="license"></a>
</p>

<p align="center">
  <a href="#why">Why</a> ·
  <a href="#quick-start">Quick Start</a> ·
  <a href="#preflight">Preflight</a> ·
  <a href="#local-bot-api-server">Local Server</a> ·
  <a href="#what-belongs-here">Boundaries</a>
</p>

---

## Why

Three FreshLab bots each carried their own HTTP client to the same API. The
transport matched function for function — `get`, `post`, `attempt`, `do`,
`redactError`, `parseAPIError`, `retryDelay` — and had already drifted: one
counted metrics, one wrapped errors differently, and only one knew about a
self-hosted Bot API server, incorrectly. A fourth bot used a library and still
hand-rolled 130 lines around it.

Then a self-hosted server two years behind answered `404 method not found` to
every rich message, and a bot went quiet for a day without a single error that
said so.

This module is the fix: one transport, one place for the local-server rules,
and a startup check that turns "silently answers nothing" into "does not
start, and says why".

## Quick Start

```go
c := tg.New(token,
    tg.WithAPIBase(cfg.TelegramAPIBase),
    tg.WithAllowedUpdates("message", "callback_query", "my_chat_member"),
    tg.WithLogger(log),
    tg.WithObserver(func(e tg.Event) {
        metrics.Telegram.Observe(e.Method, e.Status, e.Duration, e.Err)
    }),
)

me, err := c.Preflight(ctx, tg.Needs{
    Methods: []string{"sendRichMessage", "editEphemeralMessageText"},
    Wait:    30 * time.Second,
})
if err != nil {
    return err
}
log.Info("telegram ready", "username", me.Username)
```

No dependencies: logging is `*slog.Logger`, metrics are a callback, and the
`go.mod` has an empty `require` block. Nothing here decides how your bot
observes itself.

## Preflight

A Bot API server does not tell anyone its version. `getMe` says nothing about
it, the statistics port reports uptime and memory, and a containerized server
logs nothing at all. What it does answer unambiguously is whether a method
exists:

```
POST sendMessage      {}  ->  400 Bad Request: message text is empty
POST sendRichMessage  {}  ->  404 Not Found: method not found
```

`Preflight` asks for exactly the methods the bot cannot work without and
refuses to start when any are missing. Probing calls the method for real, so
only methods that fail without parameters are allowed — `getMe` would simply
run, `deleteWebhook` would drop a webhook, and `logOut` would detach the bot
from its server for ten minutes.

## Local Bot API Server

A server started with `--local` returns an absolute path from `getFile` and
serves nothing over HTTP: its `/file/bot<token>/…` route answers 404. That is
documented behavior, not a fault, and no version of the server changes it.

```go
c := tg.New(token,
    tg.WithAPIBase("http://telegram-bot-api:8081"),
    tg.WithLocalFiles("/var/lib/telegram-bot-api"),
)
```

The client then reads media from disk and removes it afterwards, because a
local server never reclaims a file it produced. It only ever touches
`<root>/<token>`, the one subdirectory that belongs to this bot — the data
directory of a shared server names every other subdirectory after another
bot's token, so mounting the whole thing would hand those over. See
[docs/local-server.md](docs/local-server.md).

## What Belongs Here

| Here | In the bot |
| --- | --- |
| transport, retries, backoff, `Retry-After` | what counts as speech, or as spam |
| token redaction in every error path | i18n and user-facing strings |
| `APIError` and its classifiers | panels, menus, callback routing |
| protocol types | domain types (delivery bookkeeping, jobs) |
| methods the family uses, plus `Call` for the rest | formatting decisions |
| rich messages and the 10.3 ephemeral contract | when to split a message |
| files, including `--local` | how long to keep a transcript |
| capability probe and preflight | metrics naming (the module emits `Event`) |

If a change would make the module know something about one bot's product, it
belongs in that bot.

## License

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
