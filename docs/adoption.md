# Adopting tg in a bot

The migration order is by risk: voicy (alpha, one user), then makeitMD, then
branchy, then searchy. One bot per release.

## 1. Depend on it

```sh
go get github.com/FreshLabDev/tg@v0.0.1-alpha.1
```

The module is public, so no `GOPRIVATE` is needed.

## 2. Replace the internal client

Bots that already talk to their client through an interface (voicy's
`bot.Telegram`) only change the types the interface names: `telegram.Update`
becomes `tg.Update`, and `*tg.Client` satisfies the interface as it stands.

Two things do not come across, by design:

- **Helpers on protocol types.** Go cannot define a method on another package's
  type, so `msg.Media()` becomes a function in the bot — `media.Of(msg)` —
  which is where the decision belonged anyway: the order in which attachments
  are preferred is a product call.
- **Domain types that live in the same package.** makeitMD's
  `telegram.DeliveryAttempt` and `telegram.Result` describe delivery
  bookkeeping, not the API. They move to `internal/db`, and only `Message`
  comes from the module. Check the JSON shape of existing rows before shipping.

Bots whose bot logic shares a package with the client (branchy's
`internal/telegram/bot.go`) should move that package first, in a commit that
changes nothing else, and adopt the module in the next one.

## 3. Add the preflight

Replace ad-hoc startup checks — a hand-rolled `getMe` with retry, a
`deleteWebhook` call, a "is the server up yet" loop — with one `Preflight`.
List the methods the bot genuinely cannot work without; a bot that only sends
plain messages needs none, and passes an empty `Methods`.

## 4. Wire the observer

```go
tg.WithObserver(func(e tg.Event) {
    if e.Err != nil && e.Status == http.StatusTooManyRequests {
        metrics.TelegramLimited.Inc(e.Method)
        return
    }
    if e.Err != nil {
        metrics.TelegramErrors.Inc(e.Method)
    }
})
```

Bots that had no Telegram metrics get them here for the first time; the names
stay each bot's own choice.

## 5. Delete the old package

`internal/telegram` goes away entirely, tests included — the equivalents live
in the module. Add a line to the bot's AGENTS.md so it does not come back:
*Telegram goes through `github.com/FreshLabDev/tg`; we do not keep a private
HTTP client.*
