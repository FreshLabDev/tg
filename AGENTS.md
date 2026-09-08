# AGENTS.md

Keep tg small, dependency-free, and true to the Telegram protocol only.

## Project Shape

- One Go module, `github.com/FreshLabDev/tg`, package `tg`.
- Standard library only. The `require` block stays empty — no logging,
  metrics, or HTTP library ever enters it.
- Consumers today: voicy, makeitMD, branchy, searchy.

## Boundaries

- Protocol belongs here: transport, retries, errors, types, methods, rich and
  ephemeral messages, files, capability probing.
- Product belongs in the bot: i18n, panels, what counts as speech, domain
  types, formatting, retention.
- A helper that needs to know why a bot sends something is a bot's helper. The
  test is simple: if two bots would want it to behave differently, it is not
  ours.
- New methods land in `methods.go` when a second bot wants them. One bot alone
  uses `Call`.

## Data And Security

- The bot token appears in every request URL and in every path a `--local`
  server produces. Nothing leaving this package may contain it: `redactError`
  covers errors, `redact` covers paths, and there is a test for both.
- Never log a request body or a response body: they carry user messages.
- `Probe` may only call methods that fail without parameters. Adding a method
  to `probeSafe` means asserting it does nothing on an empty body.
- File reads stay under `<filesRoot>/<token>`. A shared server's data
  directory holds other bots' tokens as directory names.

## Versioning

- SemVer per `docs/versioning.md`. The line starts at `v0.0.1-alpha.1`;
  `alpha` holds until a bot runs on it with live credentials.
- `BotAPI` in `tg.go` declares the Bot API version this module targets. Bump it
  in the same commit that adds methods from that version.
- Bots pin a version and upgrade deliberately. A breaking change to a method
  signature is a minor bump before v1.0.0 and must be listed in the changelog.

## Verification

```sh
go test -race ./...
go vet ./...
gofmt -l .
```

Tests are httptest-based and hermetic: no test may reach the network, and the
suite must stay under a second. Backoff is defeated by replacing `c.sleep`.
