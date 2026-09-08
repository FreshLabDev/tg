# Versioning

tg uses SemVer-style versions. This first line starts at `v0.0.1`. Alpha,
beta, and rc tags are reserved for changes that still need live validation.

## Version Line

```text
v0.0.1-alpha.1  the transport extracted from voicy, plus preflight and
                local-server files; no bot runs on it yet
v0.0.1-beta.1   after voicy, makeitMD and branchy run on it in production
v0.0.1-rc.1     release candidate
v0.0.1          first stable line for the family
```

After this line:

```text
v0.0.x          backward-compatible fixes on the 0.0.1 contract
v0.1.0          notable additions that stay source-compatible
v1.0.0          first stable client contract
```

Work happens on `dev`. Every pre-release and stable release is published from
`main`. The `## Unreleased` changelog section tracks what has merged to `dev`
but is not yet tagged.

## Rules

- Use `alpha` until at least one bot has run on it against live Telegram
  credentials, including one self-hosted Bot API server.
- Use `beta` once the bots that share it are all migrated.
- Use `rc` when only release-blocking fixes are expected.
- Patch versions for fixes and for methods added without touching an existing
  signature.
- Minor versions for a changed method signature while below `v1.0.0`; say so
  in the changelog, because every consumer has to be edited, not just bumped.
- No `v1.0.0` until the surface has survived one Bot API bump across all four
  consumers.

## Bot API versions

`BotAPI` in `tg.go` is the Bot API version the module is written against. It
is not a claim of full coverage — the module implements what the family uses —
but it is the version whose semantics the code assumes.

Bump it in the same commit that adds methods or fields from that version, and
name the Telegram release in the changelog entry. CI compares the constant
with Telegram's published version weekly, so falling behind is noticed by a
job rather than by a user whose message went unanswered.

## Consumers

Bots pin an exact version and upgrade deliberately, one at a time. That is the
whole safety story for a shared module: a bad release reaches one bot, not
four.

Upgrade order matches the migration order — voicy first (alpha, one user),
then makeitMD, then branchy, then searchy.
