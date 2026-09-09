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

- Work on `dev`. Pre-releases (`-alpha.N`, `-beta.N`, `-rc.N`) are tagged on
  `dev`; stable versions are tagged on `main`, on the merge commit from `dev`.
  The test bot runs `dev`, the production bot runs `main`.
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

## Deploying

Do not invent a deploy. [`docs/releases.md`](docs/releases.md) has a **Deploying**
section describing this stack exactly: which host directory it lives in, which
env file names the image, which networks it needs, and how to roll back. Read it
before touching anything on the host.

Two rules that hold everywhere and are easy to get wrong:

- **Nothing is built on the host.** A production stack pulls the image the
  release workflow published. A `build:` section in a production manifest is a
  bug.
- **Pin the digest, not the tag.** A tag moves; a digest names one build that was
  tested, and a rollback becomes one line with nothing to rebuild.
