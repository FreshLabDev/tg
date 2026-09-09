# Running against a self-hosted Bot API server

A Bot API server started with `--local` (`TELEGRAM_LOCAL=1` in the common
container images) lifts the cloud API's limits: downloads without a size cap,
uploads up to 2000 MB. It changes two things a bot must handle.

## getFile returns a path, not a URL

Telegram's own server returns a relative `file_path` and serves it over HTTPS.
A `--local` server returns an absolute path on its own filesystem and **serves
nothing over HTTP** — its `/file/bot<token>/…` route answers 404 for both the
relative and the absolute form. This is documented upstream: in local mode a
bot receives "the absolute local path … without the need to download the file
after a getFile request".

So the directory has to be reachable from the bot:

```yaml
services:
  bot:
    volumes:
      # Long syntax, not "source:target": a bot token contains a colon, and
      # the short form splits on colons ("too many colons").
      - type: bind
        source: ${BOT_API_DIR}/${TELEGRAM_BOT_TOKEN}
        target: /var/lib/telegram-bot-api/${TELEGRAM_BOT_TOKEN}
        # Not decoration: without a bind option Compose flattens the long
        # syntax back into "source:target:rw" and the daemon splits it on the
        # colons in the token.
        bind:
          propagation: rprivate
    user: "101:101"
```

Four details that are easy to get wrong:

- **Mount only this bot's subdirectory.** The server's data directory holds one
  subdirectory per bot, named after that bot's full token. Mounting the parent
  hands every other bot's credentials to this one.
- **Use the long volume syntax, with a `bind` option.** Both paths contain the
  token, the token contains a colon, and `source:target` splits on colons —
  and so does long syntax with no `bind` option, because Compose flattens that
  back into a string.
- **Create the directory first, owned by that uid.** Docker creates a missing
  bind source as `root`, and then neither the server nor the bot can write in
  it. `Preflight` with `Files: true` catches this at startup, but only after
  the container is already restarting.
- **Match the uid.** The server writes as uid 101 with mode 0750. A bot running
  as another user cannot read the file, and cannot delete it afterwards — which
  it must, because a local server never reclaims what it produced.

Then:

```go
c := tg.New(token,
    tg.WithAPIBase("http://telegram-bot-api:8081"),
    tg.WithLocalFiles("/var/lib/telegram-bot-api"),
)
_, err := c.Preflight(ctx, tg.Needs{Files: true})
```

`Preflight` with `Files: true` checks the mount at startup instead of letting
the first voice message discover it.

## The server pins your Bot API version

The cloud endpoint is always current. A self-hosted one is exactly as current
as the image you pinned, and pins do not move on their own. A server behind the
bot answers `404 method not found` for everything it has not learned, which for
a bot that sends rich messages means total silence.

List those methods in `Preflight` and the bot will refuse to start instead:

```go
_, err := c.Preflight(ctx, tg.Needs{
    Methods: []string{"sendRichMessage", "editEphemeralMessageText"},
    Files:   true,
    Wait:    30 * time.Second,
})
```

`Wait` covers the startup race: a self-hosted server shares a lifecycle with
the bot and often loses it by a few seconds.

## Moving a bot between servers

A token is logged in on one server at a time. Moving it from the cloud to a
local server, or back, requires `logOut` on the server it is leaving; Telegram
then refuses the cloud endpoint for ten minutes. `Client.LogOut` exists for
that, and is never called by anything else — in particular never by `Probe`.
